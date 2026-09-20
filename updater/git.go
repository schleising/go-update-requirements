package updater

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/mod/semver"
)

const gitListTagsTimeout = 30 * time.Second

var (
	// git+https://host/path, git+https://user:token@host/path, git+ssh://git@host/path
	gitURLWithRef = regexp.MustCompile(`^(git\+(?:https?|ssh|git)://(?:[^@/]+@)?[^@#]+)(?:@([^#]+))?$`)
	// git+git@github.com:user/repo.git@ref
	gitSCPWithRef = regexp.MustCompile(`^(git\+git@[^:]+:[^@#]+)(?:@([^#]+))?$`)
)

type gitRequirement struct {
	name     string
	gitURL   string // git+ URL without @ref or fragment
	ref      string
	fragment string // including leading #, or empty
}

func (g gitRequirement) repoURL() string {
	return strings.TrimPrefix(g.gitURL, "git+")
}

func (g gitRequirement) String() string {
	line := g.name + " @ " + g.gitURL
	if g.ref != "" {
		line += "@" + g.ref
	}
	return line + g.fragment
}

func parseGitRequirement(line string) (gitRequirement, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return gitRequirement{}, false
	}

	name, rest, ok := strings.Cut(line, " @ ")
	if !ok || !strings.HasPrefix(rest, "git+") {
		return gitRequirement{}, false
	}

	fragment := ""
	urlPart, frag, hasFrag := strings.Cut(rest, "#")
	if hasFrag {
		fragment = "#" + frag
	}

	gitURL, ref, ok := splitGitURLRef(urlPart)
	if !ok {
		return gitRequirement{}, false
	}

	return gitRequirement{
		name:     name,
		gitURL:   gitURL,
		ref:      ref,
		fragment: fragment,
	}, true
}

func splitGitURLRef(urlPart string) (gitURL, ref string, ok bool) {
	if matches := gitSCPWithRef.FindStringSubmatch(urlPart); matches != nil {
		return matches[1], matches[2], true
	}
	if matches := gitURLWithRef.FindStringSubmatch(urlPart); matches != nil {
		return matches[1], matches[2], true
	}
	return "", "", false
}

func normalizePackageName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func tagSemver(tag string) string {
	version := tag
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if !semver.IsValid(version) {
		return ""
	}
	return semver.Canonical(version)
}

func pickLatestTag(tags []string) (string, error) {
	var bestStable, bestPre string
	var bestStableV, bestPreV string

	for _, tag := range tags {
		version := tagSemver(tag)
		if version == "" {
			continue
		}
		if semver.Prerelease(version) == "" {
			if bestStableV == "" || semver.Compare(bestStableV, version) < 0 {
				bestStable = tag
				bestStableV = version
			}
			continue
		}
		if bestPreV == "" || semver.Compare(bestPreV, version) < 0 {
			bestPre = tag
			bestPreV = version
		}
	}

	if bestStable != "" {
		return bestStable, nil
	}
	if bestPre != "" {
		return bestPre, nil
	}
	return "", fmt.Errorf("no version tags found")
}

func listRemoteTags(repoURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitListTagsTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", repoURL)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git ls-remote %s: %w: %s", repoURL, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git ls-remote %s: %w", repoURL, err)
	}

	var tags []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		tag, found := strings.CutPrefix(strings.TrimSpace(ref), "refs/tags/")
		if !found || tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// gitListTags is the tag lister used by updateGitDependencies. Tests replace it.
var gitListTags = listRemoteTags

func displayRef(ref string) string {
	if ref == "" {
		return "unpinned"
	}
	return ref
}

func updateGitDependencies(filename string) (map[string]gitRequirement, error) {
	lines, err := readLines(filename)
	if err != nil {
		return nil, err
	}

	pins := make(map[string]gitRequirement)
	changed := false
	foundGit := false

	for i, line := range lines {
		req, ok := parseGitRequirement(line)
		if !ok {
			continue
		}

		if !foundGit {
			color.Blue("Updating git dependencies")
			foundGit = true
		}

		tags, err := gitListTags(req.repoURL())
		if err != nil {
			return nil, err
		}

		tag, err := pickLatestTag(tags)
		if err != nil {
			color.Yellow("Warning: %s has no version tags; leaving existing git ref", req.name)
			continue
		}

		if req.ref == tag {
			color.Green("%s already at latest tag %s", req.name, tag)
		} else {
			color.Green("%s: %s -> %s", req.name, displayRef(req.ref), tag)
			changed = true
		}

		req.ref = tag
		lines[i] = req.String()
		pins[normalizePackageName(req.name)] = req
	}

	if changed {
		if err := writeLines(filename, lines); err != nil {
			return nil, err
		}
	}

	return pins, nil
}

func applyGitTags(filename string, pins map[string]gitRequirement) error {
	if len(pins) == 0 {
		return nil
	}

	lines, err := readLines(filename)
	if err != nil {
		return err
	}

	changed := false
	for i, line := range lines {
		req, ok := parseGitRequirement(line)
		if !ok {
			continue
		}

		pin, ok := pins[normalizePackageName(req.name)]
		if !ok || req.ref == pin.ref {
			continue
		}

		req.ref = pin.ref
		if req.fragment == "" {
			req.fragment = pin.fragment
		}
		lines[i] = req.String()
		changed = true
	}

	if !changed {
		return nil
	}
	return writeLines(filename, lines)
}

func readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func writeLines(filename string, lines []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

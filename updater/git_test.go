package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitRequirement(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   gitRequirement
		wantOK bool
	}{
		{
			name: "https sha",
			line: "md_mermaid @ git+https://github.com/schleising/md_mermaid@e2076ca7bff9f7bb27ce73549e604d742de8af1c",
			want: gitRequirement{
				name:   "md_mermaid",
				gitURL: "git+https://github.com/schleising/md_mermaid",
				ref:    "e2076ca7bff9f7bb27ce73549e604d742de8af1c",
			},
			wantOK: true,
		},
		{
			name: "https sha with subdirectory",
			line: "notify-run @ git+https://github.com/notify-run/notify.run.git@0b37558d54fda52d382a72cef05c05856c605c7b#subdirectory=py_client",
			want: gitRequirement{
				name:     "notify-run",
				gitURL:   "git+https://github.com/notify-run/notify.run.git",
				ref:      "0b37558d54fda52d382a72cef05c05856c605c7b",
				fragment: "#subdirectory=py_client",
			},
			wantOK: true,
		},
		{
			name: "https with credentials and tag",
			line: "pkg @ git+https://user:token@github.com/org/pkg.git@v1.2.3",
			want: gitRequirement{
				name:   "pkg",
				gitURL: "git+https://user:token@github.com/org/pkg.git",
				ref:    "v1.2.3",
			},
			wantOK: true,
		},
		{
			name: "ssh with tag",
			line: "pkg @ git+ssh://git@github.com/org/pkg.git@v1.2.3",
			want: gitRequirement{
				name:   "pkg",
				gitURL: "git+ssh://git@github.com/org/pkg.git",
				ref:    "v1.2.3",
			},
			wantOK: true,
		},
		{
			name: "scp-like with tag",
			line: "pkg @ git+git@github.com:org/pkg.git@v1.2.3",
			want: gitRequirement{
				name:   "pkg",
				gitURL: "git+git@github.com:org/pkg.git",
				ref:    "v1.2.3",
			},
			wantOK: true,
		},
		{
			name: "unpinned https",
			line: "pkg @ git+https://github.com/org/pkg.git",
			want: gitRequirement{
				name:   "pkg",
				gitURL: "git+https://github.com/org/pkg.git",
			},
			wantOK: true,
		},
		{
			name:   "pypi pin",
			line:   "Django==4.2.0",
			wantOK: false,
		},
		{
			name:   "comment",
			line:   "# md_mermaid @ git+https://example.com/repo@abc",
			wantOK: false,
		},
		{
			name:   "blank",
			line:   "  ",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGitRequirement(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPickLatestTag(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		want    string
		wantErr bool
	}{
		{
			name: "highest stable semver",
			tags: []string{"v0.1.0", "v1.2.0", "v1.10.0", "v1.2.1"},
			want: "v1.10.0",
		},
		{
			name: "preserves missing v prefix",
			tags: []string{"0.9.0", "1.0.0", "1.2.0"},
			want: "1.2.0",
		},
		{
			name: "prefers stable over newer prerelease",
			tags: []string{"v1.9.0", "v2.0.0-rc.1", "v2.0.0-alpha"},
			want: "v1.9.0",
		},
		{
			name: "prerelease when only option",
			tags: []string{"v2.0.0-alpha", "v2.0.0-rc.1"},
			want: "v2.0.0-rc.1",
		},
		{
			name: "ignores non-semver tags",
			tags: []string{"nightly", "v1.0.0", "release"},
			want: "v1.0.0",
		},
		{
			name:    "no version tags",
			tags:    []string{"nightly", "release"},
			wantErr: true,
		},
		{
			name:    "empty",
			tags:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickLatestTag(tt.tags)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateGitDependencies(t *testing.T) {
	original := gitListTags
	t.Cleanup(func() { gitListTags = original })

	gitListTags = func(repoURL string) ([]string, error) {
		switch {
		case strings.Contains(repoURL, "md_mermaid"):
			return []string{"v0.1.0", "v1.2.3"}, nil
		case strings.Contains(repoURL, "notify.run"):
			return []string{"0.0.1"}, nil
		case strings.Contains(repoURL, "untagged"):
			return []string{"nightly"}, nil
		default:
			return nil, fmt.Errorf("unexpected repo %s", repoURL)
		}
	}

	path := filepath.Join(t.TempDir(), "requirements.txt")
	input := strings.Join([]string{
		"Django==4.2.0",
		"md_mermaid @ git+https://github.com/schleising/md_mermaid@e2076ca7bff9f7bb27ce73549e604d742de8af1c",
		"notify-run @ git+https://github.com/notify-run/notify.run.git@0b37558d54fda52d382a72cef05c05856c605c7b#subdirectory=py_client",
		"untagged @ git+https://github.com/example/untagged@abc123",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	pins, err := updateGitDependencies(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := pins["untagged"]; ok {
		t.Fatal("untagged repo should not be pinned to a version tag")
	}
	if pins["md-mermaid"].ref != "v1.2.3" {
		t.Fatalf("md_mermaid ref=%q", pins["md-mermaid"].ref)
	}
	if pins["notify-run"].ref != "0.0.1" {
		t.Fatalf("notify-run ref=%q", pins["notify-run"].ref)
	}
	if pins["notify-run"].fragment != "#subdirectory=py_client" {
		t.Fatalf("notify-run fragment=%q", pins["notify-run"].fragment)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Django==4.2.0",
		"md_mermaid @ git+https://github.com/schleising/md_mermaid@v1.2.3",
		"notify-run @ git+https://github.com/notify-run/notify.run.git@0.0.1#subdirectory=py_client",
		"untagged @ git+https://github.com/example/untagged@abc123",
	}, "\n") + "\n"
	if string(got) != want {
		t.Fatalf("file mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestApplyGitTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requirements.txt")
	frozen := strings.Join([]string{
		"Django==5.0.0",
		"md_mermaid @ git+https://github.com/schleising/md_mermaid@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"notify-run @ git+https://github.com/notify-run/notify.run.git@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb#subdirectory=py_client",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(frozen), 0644); err != nil {
		t.Fatal(err)
	}

	pins := map[string]gitRequirement{
		"md-mermaid": {
			name:   "md_mermaid",
			gitURL: "git+https://github.com/schleising/md_mermaid",
			ref:    "v1.2.3",
		},
		"notify-run": {
			name:     "notify-run",
			gitURL:   "git+https://github.com/notify-run/notify.run.git",
			ref:      "0.0.1",
			fragment: "#subdirectory=py_client",
		},
	}

	if err := applyGitTags(path, pins); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Django==5.0.0",
		"md_mermaid @ git+https://github.com/schleising/md_mermaid@v1.2.3",
		"notify-run @ git+https://github.com/notify-run/notify.run.git@0.0.1#subdirectory=py_client",
	}, "\n") + "\n"
	if string(got) != want {
		t.Fatalf("file mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestGitRequirementString(t *testing.T) {
	req := gitRequirement{
		name:     "notify-run",
		gitURL:   "git+https://github.com/notify-run/notify.run.git",
		ref:      "0.0.1",
		fragment: "#subdirectory=py_client",
	}
	want := "notify-run @ git+https://github.com/notify-run/notify.run.git@0.0.1#subdirectory=py_client"
	if got := req.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizePackageName(t *testing.T) {
	if got := normalizePackageName("md_mermaid"); got != "md-mermaid" {
		t.Fatalf("got %q", got)
	}
}

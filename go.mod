module schleising.net/update-requirements

go 1.22.1

replace schleising.net/updater => ./updater

require (
	github.com/fatih/color v1.18.0
	schleising.net/updater v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.26.0 // indirect
)

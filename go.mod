module schleising.net/update-requirements

go 1.27.1

replace schleising.net/updater => ./updater

require (
	github.com/fatih/color v1.19.0
	schleising.net/updater v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

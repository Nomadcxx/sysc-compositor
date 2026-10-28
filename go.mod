module github.com/Nomadcxx/sysc-compositor

go 1.26.4

require (
	github.com/go-text/typesetting v0.3.5
	github.com/godbus/dbus/v5 v5.2.2
	github.com/sblinch/kdl-go v0.0.0-20260121213736-8b7053306ca6
	golang.org/x/image v0.47.0
)

require golang.org/x/sys v0.49.0 // indirect

replace github.com/godbus/dbus/v5 => ./third_party/godbus

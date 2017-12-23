This is a systemd socket-activated version of godoc's HTTP mode.

If you'd like to use godoc's built-in HTTP server to browse Go documentation offline,
you can use this package instead to make it start lazily, the first time it's accessed, and
to make it shutdown automatically after period of inactivity.

It imports `golang.org/x/tools/godoc` package, so the interface is exactly the same.

However, because of the fact that some logic is kept privately in `golang.org/x/tools/cmd/godoc`
package, in order to avoid copy-pasting too much code, I decided to skip that functionality. As a result,
blog page, playground and codewalk don't work. If you need them, feel free to open an issue.

## Installation

This instruction assumes you want to install the binary for your current user,
and to use the user systemd instance.

If you want to install it globally, you'll need to run different commands and edit
systemd unit files as necessary.

1. Install it to your `GOPATH` with `go install github.com/WGH-/socket-activated-godoc`
2. Put systemd units into `~/.config/systemd/user` directory: `cp $GOPATH/src/github.com/WGH-/socket-activated-godoc/godoc.{service,socket} ~/.config/systemd/user`, edit them (e.g. listening address) as needed.
3. Reload systemd state and start the socket: `systemctl --user daemon-reload && systemctl --user enable --now godoc.socket`

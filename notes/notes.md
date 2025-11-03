ixcellent! I've fixed the TTY issue that was causing the inject loop. Here's what was done:

  Root Cause

  The problem was on pkg/nomad/nomad.go:159 where TTY was hardcoded to true:
  return n.client.Allocations().Exec(ctx, alloc, task, true, ...)

  When TTY is enabled, the terminal echoes input and adds control characters, which corrupts DevPod's inject protocol that transfers the agent binary via
  stdin/stdout.

  The Fix

  1. Added import for terminal detection (pkg/nomad/nomad.go:16)

  dockerterm "github.com/moby/term"

  2. Dynamic TTY detection (pkg/nomad/nomad.go:160-163)

  // Detect if stdin is a terminal to determine TTY allocation
  // When DevPod injects its agent, stdin is a pipe (not a terminal)
  // When running interactive commands, stdin is a real terminal
  _, isTTY := dockerterm.GetFdInfo(stdin)

  3. Use detected value instead of hardcoded true (pkg/nomad/nomad.go:165)

  return n.client.Allocations().Exec(ctx, alloc, task, isTTY, ...)

  How This Works

  - During inject: stdin is a pipe → isTTY = false → clean binary transfer works
  - Interactive shells: stdin is a terminal → isTTY = true → proper terminal emulation

  Rebuild and Test

  Now rebuild the provider:
  ./hack/build.sh

  Then reinstall:
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  And test:
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  The "unexpected message during inject: pong" error should now be resolved! 🎉


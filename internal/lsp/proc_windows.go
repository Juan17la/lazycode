package lsp

import "os/exec"

func prepare(cmd *exec.Cmd) {}

// ponytail: only the server itself is killed, so a helper it started can stay behind. A job object
// would end the whole tree.
func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill() //nolint:errcheck // It may be gone already.
	}
}

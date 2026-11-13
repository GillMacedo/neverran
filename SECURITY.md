# Security

Report vulnerabilities through [GitHub's private vulnerability reporting](https://github.com/GillMacedo/neverran/security/advisories/new), not in a public issue.

`neverran` only reads the files you pass it and the source tree in `--dir`. It doesn't run your tests or code, connect to CI or the network, or write files. Test names, package paths and `t.Skip` messages are printed in findings, so don't share its output if those hold anything private.

A missed or wrong finding is a normal bug. If `neverran` writes a file, opens a network connection, runs code from the tree, or reports a clean result for input it couldn't read, report it privately. `neverran` is a diagnostic aid and doesn't enforce anything.

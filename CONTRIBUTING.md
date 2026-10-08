# Contributing

Bug reports and pull requests are welcome.

```sh
make test       # go vet + unit tests are required to pass
make fmt-check  # gofmt
make lint       # shellcheck (pip install shellcheck-py, or apt install shellcheck)
make secrets    # scans the tree for keys and tokens
make packages   # builds everything into dist/
```

- Keep the agent dependency-free (standard library only) and the shell scripts POSIX `sh`.
- User-visible changes: add a line to `CHANGELOG.md` and update both language versions of the docs (`*.md` and `*.ru.md`).
- Never commit keys, tokens or real hostnames/IP addresses of your site; use the `examples/` conventions.

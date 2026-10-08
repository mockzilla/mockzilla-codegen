# Contributing

Issues and pull requests are welcome. For a larger change, open an issue first so we can agree on
the shape before you write it.

## Setup

Go 1.26 or newer. `make help` lists every target.

```sh
make test PKG=./internal/naming RUN=TestIdent   # one test while you work
make check                                      # lint, 100% coverage gate, tidy, examples
make examples                                   # regenerate the golden examples in examples/
make generate                                   # examples and config.schema.json
make test-integration SPEC=3.0/misc/<spec>.yml  # one real-world spec, end to end
```

`make check` has to pass before you open a pull request. The full integration run covers 2,200+
specs and takes a while; run one spec, or `make test-integration-ci` for the set CI uses.

## Rules

[AGENTS.md](AGENTS.md) has the layout of the repo and its rules. They apply to people too. In short:

- Every `.go` file starts with the MIT license header. Copy it from any file.
- Library code returns errors and diagnostics. Only `pkg/cli` prints, to the writers it is given.
- Output is deterministic: the same spec and config give the same bytes on every run.
- Templates hold no logic beyond `range` and `if` on precomputed fields.
- Golden files change only through `UPDATE=1` runs, never by hand.
- Every line of new code is covered by a test. Exceptions go in `.covignore`.

## Adding a router

[docs/frameworks.md](docs/frameworks.md) says what a framework package implements and how to test
it.

## Pull requests

One topic per pull request, based on `main`. Say what was wrong and what the change does. By
contributing you agree that your work is licensed under the [MIT license](LICENSE).

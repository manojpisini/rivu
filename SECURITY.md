# Security policy

## Reporting a vulnerability

**Do not open a public issue for security vulnerabilities.**

Report privately through GitHub Security Advisories:

https://github.com/manojpisini/rivu/security/advisories/new

Include:

- what the issue is and how to reproduce it,
- the version (`rivu version --json`),
- impact (especially: can it read, move or delete files the user did
  not intend?).

You should get an acknowledgement within a few days. Once a fix ships
in a release, credit is offered unless you prefer to stay anonymous.

## Supported versions

| Version | Supported |
|---|---|
| latest release | ✅ |
| `master` (unreleased) | best effort |

## Security-relevant design commitments

These are invariants of the product; a PR that breaks one is a security
bug even if it "looks intentional":

1. **Rivu never deletes a project folder.** The only removals are
   rollback of files/dirs the current run itself created, through the
   allow-listed helper (a CI test enforces this).
2. **Secrets are name-only.** `.env*`, `*.pem`, `id_rsa*`, `*.key`,
   `secrets.*` are listed in the Map by name; their contents are never
   read, printed or logged.
3. **Plan → confirm → Apply.** Mutating commands need confirmation;
   `--yes` is the explicit non-TTY bypass; scripts without it exit 4.
4. **No shell interpolation.** External commands run via
   `exec.Command(name, args...)`, never `sh -c` with user input.
5. **Paths are contained.** User names pass through `internal/slug`;
   destinations pass `pathsafe.Contained`.

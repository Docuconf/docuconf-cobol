# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately, through GitHub's private vulnerability reporting: open the repository's
**Security** tab and choose **Report a vulnerability**
([direct link](https://github.com/docuconf/docuconf-cobol/security/advisories/new)). Do not open a public issue, pull
request or discussion for a suspected vulnerability.

Include what you can of:

- the affected component and version (`docuconf-cobol` version or image digest, and the GnuCOBOL version the loader
  was compiled with);
- what an attacker can do, and what they need first;
- steps or a minimal copybook, environment or program that reproduces it.

We work on the fix in a private security advisory, credit you in it unless you prefer otherwise, and publish the
advisory when a fixed release is out.

## Response targets

| | |
|---|---|
| Acknowledge the report | within 3 business days |
| First assessment (confirmed or not, severity) | as soon as we can reproduce it, and we keep you updated in the advisory |
| Fix | released as a patch to the supported version, then the advisory is published |

## Supported versions

Security fixes go to the latest minor release, as a new patch release (see [RELEASING.md](RELEASING.md)). A fix in
the loader's runtime reaches a program only when its loader is regenerated (or, with `-runtime copy`, when the
`DCRTWS` and `DCRTPD` copybooks are replaced) and the program recompiled.

**During the beta, only the latest release is supported.** Upgrade to it to get a fix.

## Scope

In scope:

- the `docuconf-cobol` generator, including its release binaries and container image;
- the loaders it generates and the runtime copybooks in [`copybooks`](copybooks), for example a value that passes the
  loader but should not, or a secret's value printed in a problem or warning.

Out of scope: the example application under [`examples`](examples), vulnerabilities in dependencies that
docuconf-cobol does not make reachable (report those upstream), and issues in a platform or cluster that only arise
from its own misconfiguration. The `docuconf` CLI (`docuconf exec`), the Go SDK, the Helm chart and the spec live in
[docuconf-go](https://github.com/docuconf/docuconf-go) and follow its policy; other language SDKs live in their own
repositories.

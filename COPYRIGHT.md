# Copyright

Copyright © 2026 Stefano Balzarotti (OrbintSoft) and contributors.

## Licence

emergup is free software: you can redistribute it and/or modify it under the
terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version (`GPL-3.0-or-later`).

emergup is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE. See the GNU General Public License for more details.

The full licence text is in [LICENSE](LICENSE). The list of contributors is in
[AUTHORS.md](AUTHORS.md).

## Contributions

Contributors keep the copyright in their contributions and license them under
the same terms (`GPL-3.0-or-later`). Every commit carries a `Signed-off-by`
trailer certifying the [Developer Certificate of Origin](DCO.txt); see
[CONTRIBUTING.md](CONTRIBUTING.md).

## Third-party components

Compiled into the distributed binary:

- **Go standard library** — BSD-3-Clause.
- **[go-toml](https://github.com/pelletier/go-toml)**
  (`github.com/pelletier/go-toml/v2`), which reads the configuration files —
  MIT.

Used only by the tests, not compiled into the binary:

- **[testify](https://github.com/stretchr/testify)** (`github.com/stretchr/testify`)
  — MIT. It bundles copies of **go-difflib** (BSD-3-Clause) and **go-spew**
  (ISC).
- **[yaml](https://github.com/yaml/go-yaml)** (`go.yaml.in/yaml/v3`), required
  by testify — MIT and Apache-2.0.

Every further dependency is listed here, with its licence, when it is added.

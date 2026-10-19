# Third-party notices

PS4 Games Server is licensed GPL-2.0-only; see LICENSE. Copyright 2026 contributors.

Behavioral reference: [e1z0/FPKGi-node-server](https://github.com/e1z0/FPKGi-node-server), commit 88a969cf991303c36be64438a6752955c46b578d. Native code was written for this repository; no Orbis binaries, game packages, original icons, Node dependencies or original desktop assets are distributed. Synthetic fixtures contain no game content.

The compiled executable uses the Go standard library, distributed under the Go BSD-style license: https://go.dev/LICENSE. Go's runtime and standard library include their own third-party notices: https://go.dev/src/LICENSE and https://go.dev/src/vendor/modules.txt. A copy of the Go license is included below. Docker images additionally contain Alpine Linux and CA certificates under their respective licenses. Windows service integration additionally links golang.org/x/sys v0.30.0 (BSD-3-Clause, Copyright 2009 The Go Authors), using the same Go Authors license reproduced below. It is compiled into the EXE; users need no additional runtime. Source and license: https://go.googlesource.com/sys/+/refs/tags/v0.30.0/LICENSE.

Optional metadata and screenshot responses are supplied by IGDB/Twitch; they are not bundled. API terms and attribution requirements apply to that data. The API integration follows the [official IGDB documentation](https://api-docs.igdb.com/). PS4, PlayStation and game titles belong to their respective owners.

## Go license

Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

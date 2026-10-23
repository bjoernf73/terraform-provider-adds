# Third-party notices

This provider embeds third-party source code. The full license text for each component is
reproduced below, as required by the respective licenses.

---

## GPRegistryPolicyParser

An adapted copy of Microsoft's **GPRegistryPolicyParser** is embedded at
[`internal/ad/scripts/gpregistrypolicyparser.ps1`](internal/ad/scripts/gpregistrypolicyparser.ps1)
and used by the `adds_json_gpo` resource and the `adds_json_gpo_export` data source to read and
write Group Policy `Registry.pol` files. It has been adapted to run as an embedded script (see
the header comment in that file for the specific modifications); the parsing logic is unchanged.

- Project: https://github.com/PowerShell/GPRegistryPolicyParser (archived, read-only)
- License: MIT

```
PowerShell-GPRegistryPolicy-Cmdlets v.0.1

Copyright (c) Microsoft Corporation

All rights reserved.

MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy of this software
and associated documentation files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or
substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING
BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

# PawnIO modules

The two signed PawnIO modules the sensor helper loads to read the
processor's temperature (spec 103), unchanged from PawnIO.Modules' release
0.2.12 (`release_0_2_12.zip`, SHA-256
`09698897ce19b92f1043a2e322cfcf0b1fe823e0b576971b178a65244fb0af9d`):

| File | SHA-256 |
|---|---|
| `AMDFamily17.bin` | `dae74615761b78bdf064dfb3e136252ddcc6fc727d88f14738d0e5800d427a91` |
| `IntelMSR.bin` | `30a5a302283de1e080dfee99e686eca91d7e53e911095cb91a320fb961f1ce12` |

They are LGPL-2.1 (`COPYING`); their source is
<https://github.com/namazso/PawnIO.Modules>. PawnIO's driver checks each
module's signature before it loads it. `modules_test.go` pins the hashes.

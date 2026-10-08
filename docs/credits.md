# Credits

hayami is a port of two Python programs by the same author, and every device
it reads was learned from someone else's open-source work before any of it
was written. This page says whose.

## The protocols

The device code was written here first (specs 006, 008, 009, 015–018) and
moved to [sanshoku](https://github.com/ushineko/sanshoku) in spec 020, whose
[credits](https://github.com/ushineko/sanshoku/blob/main/docs/credits.md)
carry the detail. In short:

| Project | Licence | What it taught |
|---|---|---|
| [Solaar](https://github.com/pwr-Solaar/Solaar) | GPL-2.0 | HID++ over hidraw: features, registers, error forms, the software-ID convention that lets two programs share a receiver. Spec 008 replaced a `solaar show` subprocess that took 3.5 s with a read that takes 5 ms. |
| [OpenRazer](https://github.com/openrazer/openrazer) | GPL-2.0 | The Razer report format and the dock's RF relay transaction ID (spec 016). |
| [rivalcfg](https://github.com/flozz/rivalcfg) | WTFPL | The SteelSeries battery commands and product table (spec 017). |
| [LibrePods](https://github.com/librepods-org/librepods) | GPL-3.0 | Apple's accessory protocol over L2CAP for AirPods (spec 009). |
| [HeadsetControl](https://github.com/Sapd/HeadsetControl) | GPL-3.0 | The Arctis Nova Pro Wireless base station, read through it as a subprocess until spec 020, then directly. |
| [liquidctl](https://github.com/liquidctl/liquidctl) | GPL-3.0 | The Kraken, read through it as a subprocess until spec 020; its protocol work is what the direct driver was learned from. |
| The Linux kernel | GPL-2.0 | hidraw, hwmon by label, BlueZ over D-Bus, `/proc/net/dev`. |

## Built on

| Library | Licence | For |
|---|---|---|
| [Fyne](https://github.com/fyne-io/fyne) | BSD-3-Clause | The desktop panel and the preferences window. |
| [fynedesygn](https://github.com/ushineko/fynedesygn) | MIT | The glance window, the cards, cells, meters and sparklines, the theme. |
| [Bubble Tea](https://github.com/charmbracelet/bubbletea) and Lip Gloss | MIT | The terminal panel. |
| [sanshoku](https://github.com/ushineko/sanshoku) | MIT | Every device. |
| [Cobra](https://github.com/spf13/cobra) | Apache-2.0 | The command line. |
| [mdlayher/wifi](https://github.com/mdlayher/wifi) | MIT | A Wi-Fi interface's signal, frequency and link rates over nl80211 on Linux (spec 037). |

## Where it came from

`ag-scripts/peripheral-battery-monitor` (PyQt6) is the behavioural reference
for the panel, and `ag-scripts/claude-usage-widget-windows` for the usage
pane; both are the same author's and are what this program replaces.

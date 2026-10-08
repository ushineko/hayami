package panel

/*
bluetooth is no vendor at all on Windows (spec 035).

sanshoku's Bluetooth drivers read BlueZ, which is Linux's, and Apple's
accessory protocol, which needs an L2CAP socket Windows does not give a
program. Asked here they could only say "BlueZ is a Linux service" on every
poll: a doctor line about software nobody on this system could install.
Leaving them out says the true thing, that Bluetooth batteries are not read
here, which the README states.
*/
func bluetooth() []vendor { return nil }

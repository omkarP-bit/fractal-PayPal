package region

import "testing"

func TestResolveKnownRegions(t *testing.T) {
	us, ok := Resolve("us")
	if !ok || us.Currency != "USD" || us.CrossBorderOnly {
		t.Fatalf("US = %+v, ok=%v", us, ok)
	}

	in, ok := Resolve("IN")
	if !ok || in.Currency != "USD" || !in.CrossBorderOnly {
		t.Fatalf("IN = %+v, ok=%v", in, ok)
	}
	if !Allows(in, "USD") || !Allows(in, "eur") || Allows(in, "INR") {
		t.Fatalf("India currency rules are wrong: %+v", in)
	}

	de, ok := Resolve("DE")
	if !ok || de.Currency != "EUR" {
		t.Fatalf("DE = %+v, ok=%v", de, ok)
	}
}

func TestResolveUnknownRegion(t *testing.T) {
	if _, ok := Resolve("ZZ"); ok {
		t.Fatal("expected unknown region to be rejected")
	}
}

func TestAllNormalisesCurrencies(t *testing.T) {
	for _, country := range All() {
		if len(country.Currencies) == 0 {
			t.Fatalf("%s has no currencies", country.Code)
		}
		if country.Currencies[0] != country.Currency {
			t.Fatalf("%s default %q is not first in %v", country.Code, country.Currency, country.Currencies)
		}
	}
}

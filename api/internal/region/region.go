// Package region maps a merchant's country to the currency FRACTAL should use.
//
// Region is required at onboarding because silently defaulting to USD produces
// amount and currency mismatches later. PayPal has regional restrictions:
// India, for example, supports cross-border payments rather than domestic INR
// checkout, so an Indian merchant prices in a settlement currency such as USD.
package region

import (
	"sort"
	"strings"
)

// Country describes one supported merchant region.
type Country struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Currency        string   `json:"currency"`
	Currencies      []string `json:"currencies"`
	CrossBorderOnly bool     `json:"cross_border_only"`
	Note            string   `json:"note,omitempty"`
}

var countries = []Country{
	{Code: "US", Name: "United States", Currency: "USD"},
	{Code: "IN", Name: "India", Currency: "USD", Currencies: []string{"USD", "EUR", "GBP", "AUD", "CAD", "SGD", "AED"},
		CrossBorderOnly: true,
		Note:            "PayPal India handles cross-border payments only. Price export orders in a supported foreign currency; domestic INR checkout is not available."},
	{Code: "GB", Name: "United Kingdom", Currency: "GBP"},
	{Code: "CA", Name: "Canada", Currency: "CAD"},
	{Code: "AU", Name: "Australia", Currency: "AUD"},
	{Code: "NZ", Name: "New Zealand", Currency: "NZD"},
	{Code: "JP", Name: "Japan", Currency: "JPY"},
	{Code: "SG", Name: "Singapore", Currency: "SGD"},
	{Code: "CH", Name: "Switzerland", Currency: "CHF"},
	{Code: "SE", Name: "Sweden", Currency: "SEK"},
	{Code: "NO", Name: "Norway", Currency: "NOK"},
	{Code: "DK", Name: "Denmark", Currency: "DKK"},
	{Code: "PL", Name: "Poland", Currency: "PLN"},
	{Code: "CZ", Name: "Czechia", Currency: "CZK"},
	{Code: "HU", Name: "Hungary", Currency: "HUF"},
	{Code: "IL", Name: "Israel", Currency: "ILS"},
	{Code: "MX", Name: "Mexico", Currency: "MXN"},
	{Code: "BR", Name: "Brazil", Currency: "BRL"},
	{Code: "ZA", Name: "South Africa", Currency: "ZAR"},
	{Code: "AE", Name: "United Arab Emirates", Currency: "AED"},
	{Code: "HK", Name: "Hong Kong", Currency: "HKD"},
	{Code: "MY", Name: "Malaysia", Currency: "MYR"},
	{Code: "PH", Name: "Philippines", Currency: "PHP"},
	{Code: "TH", Name: "Thailand", Currency: "THB"},
}

var euroCountries = map[string]string{
	"AT": "Austria", "BE": "Belgium", "BG": "Bulgaria", "HR": "Croatia",
	"CY": "Cyprus", "EE": "Estonia", "FI": "Finland", "FR": "France",
	"DE": "Germany", "GR": "Greece", "IE": "Ireland", "IT": "Italy",
	"LV": "Latvia", "LT": "Lithuania", "LU": "Luxembourg", "MT": "Malta",
	"NL": "Netherlands", "PT": "Portugal", "RO": "Romania", "SK": "Slovakia",
	"SI": "Slovenia", "ES": "Spain",
}

func init() {
	for code, name := range euroCountries {
		countries = append(countries, Country{Code: code, Name: name, Currency: "EUR"})
	}
	sort.Slice(countries, func(i, j int) bool { return countries[i].Name < countries[j].Name })
}

// All returns the supported regions in display order.
func All() []Country {
	out := make([]Country, len(countries))
	for i, country := range countries {
		normalise(&country)
		out[i] = country
	}
	return out
}

// Resolve looks up a country code (case-insensitive).
func Resolve(code string) (Country, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, country := range countries {
		if country.Code == code {
			normalise(&country)
			return country, true
		}
	}
	return Country{}, false
}

// Allows reports whether currency is valid for the country. The first currency
// in a region's list is its default settlement currency.
func Allows(country Country, currency string) bool {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return false
	}
	for _, allowed := range country.Currencies {
		if allowed == currency {
			return true
		}
	}
	return false
}

// DefaultCurrency returns the settlement currency for a country code.
func DefaultCurrency(code string) string {
	country, ok := Resolve(code)
	if !ok {
		return ""
	}
	return country.Currency
}

func normalise(country *Country) {
	if len(country.Currencies) == 0 {
		country.Currencies = []string{country.Currency}
	}
	if country.Currency == "" && len(country.Currencies) > 0 {
		country.Currency = country.Currencies[0]
	}
}

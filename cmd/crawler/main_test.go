package main

import "testing"

func TestShouldAddToQueue(t *testing.T) {

	type inOut struct {
		in     string
		expect bool
	}

	originalHost = "www.homes.com"
	cases := []inOut{
		{"http://www.homes.com/for-sale/denver-co/", true},
		{"http://www.homes.com/property/754-e-7th-ave-denver-co-80203/id-500012484344/", true},
		{"http://www.homes.com/real-estate-agents/trudy-lovell/id-252337/", true},
		{"http://www.pornhub.com/", false},
		{"http://www.homes.com/property/754-e-7th-ave-denver-ca-80203/id-500012484344/", true},
		{"http://www.homes.com/property/754-e-7th-ave-denver-ny-80203/id-500012484344/", true},
		{"http://www.homes.com/property/754-e-7th-ave-denver-hi-80203/id-500012484344/", true},
		{"http://www.homes.com/property/754-e-7th-ave-denver-nv-80203/id-500012484344/", false},
		{"http://www.homes.com/property/754-e-7th-ave-denver-ut-80203/id-500012484344/", false},
		{"http://www.homes.com/property/754-e-7th-ave-denver-nj-80203/id-500012484344/", false},
	}

	for _, c := range cases {
		got := shouldAddToQueue(c.in)
		if got != c.expect {
			t.Errorf("shouldAddToQueue(%q) == %q, expected %q", c.in, got, c.expect)
		}
	}
}

package main

import (
	"testing"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

func TestParseDomainRule(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		rule  string
		typed routercommon.Domain_Type
		value string
	}{
		{"example.ru", routercommon.Domain_RootDomain, "example.ru"},
		{"domain:Example.RU", routercommon.Domain_RootDomain, "example.ru"},
		{"suffix:example.ru", routercommon.Domain_RootDomain, "example.ru"},
		{"full:www.example.ru", routercommon.Domain_Full, "www.example.ru"},
		{"keyword:example", routercommon.Domain_Plain, "example"},
		{`regexp:^ad[0-9]+\.example\.ru$`, routercommon.Domain_Regex, `^ad[0-9]+\.example\.ru$`},
	} {
		rule, err := parseDomainRule(testCase.rule)
		if err != nil {
			t.Fatalf("parseDomainRule(%q): %v", testCase.rule, err)
		}
		if rule.Type != testCase.typed || rule.Value != testCase.value {
			t.Fatalf("parseDomainRule(%q) = %v %q, want %v %q", testCase.rule, rule.Type, rule.Value, testCase.typed, testCase.value)
		}
	}
	for _, rule := range []string{"unknown:example.ru", "domain:", `regexp:^(`} {
		if _, err := parseDomainRule(rule); err == nil {
			t.Fatalf("parseDomainRule(%q) accepted an invalid rule", rule)
		}
	}
}

func TestExcluderCovers(t *testing.T) {
	t.Parallel()
	rules, err := parseDomainRules([]string{"domain:mail.ru", "full:vk.com", "keyword:adfox", `regexp:^ads?\..*\.ru$`})
	if err != nil {
		t.Fatal(err)
	}
	rules = append(rules, &routercommon.Domain{
		Type:      routercommon.Domain_RootDomain,
		Value:     "tagged.ru",
		Attribute: []*routercommon.Domain_Attribute{{Key: "ads"}},
	})
	excluder, err := newExcluder(rules, []string{"ads"})
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		domain *routercommon.Domain
		want   bool
	}{
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "mail.ru"}, true},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "smtp.mail.ru"}, true},
		{&routercommon.Domain{Type: routercommon.Domain_Full, Value: "a.smtp.mail.ru"}, true},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "notmail.ru"}, false},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "ru"}, false},
		{&routercommon.Domain{Type: routercommon.Domain_Full, Value: "vk.com"}, true},
		// A "full:" rule leaves the wider root domain alone, it would take
		// the subdomains down with it.
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "vk.com"}, false},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "adfox.yandex.ru"}, true},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "ads.example.ru"}, true},
		{&routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: "downloads.example.ru"}, false},
		{&routercommon.Domain{
			Type:      routercommon.Domain_RootDomain,
			Value:     "banner.example.ru",
			Attribute: []*routercommon.Domain_Attribute{{Key: "ads"}},
		}, true},
	} {
		if got := excluder.covers(testCase.domain); got != testCase.want {
			t.Fatalf("covers(%v %q) = %v, want %v", testCase.domain.Type, testCase.domain.Value, got, testCase.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	normalized := normalize([]*routercommon.Domain{
		{Type: routercommon.Domain_RootDomain, Value: "b.ru"},
		{Type: routercommon.Domain_Full, Value: "a.ru"},
		{Type: routercommon.Domain_RootDomain, Value: "a.ru"},
		{Type: routercommon.Domain_RootDomain, Value: "b.ru"},
	})
	if len(normalized) != 3 {
		t.Fatalf("normalize kept %d domains, want 3", len(normalized))
	}
	want := []string{"a.ru", "b.ru", "a.ru"}
	for index, domain := range normalized {
		if domain.Value != want[index] {
			t.Fatalf("normalize()[%d] = %q, want %q", index, domain.Value, want[index])
		}
	}
}

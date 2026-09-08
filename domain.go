package main

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

// excluder decides which domains of the direct list an exclusion rule removes.
type excluder struct {
	roots       map[string]bool
	full        map[string]bool
	keywords    []string
	expressions []*regexp.Regexp
	attributes  map[string]bool
}

func newExcluder(rules []*routercommon.Domain, attributes []string) (*excluder, error) {
	result := &excluder{
		roots:      make(map[string]bool),
		full:       make(map[string]bool),
		attributes: make(map[string]bool),
	}
	for _, attribute := range attributes {
		result.attributes[attribute] = true
	}
	for _, rule := range rules {
		switch rule.Type {
		case routercommon.Domain_RootDomain:
			result.roots[rule.Value] = true
		case routercommon.Domain_Full:
			result.full[rule.Value] = true
		case routercommon.Domain_Plain:
			result.keywords = append(result.keywords, rule.Value)
		case routercommon.Domain_Regex:
			expression, err := regexp.Compile(rule.Value)
			if err != nil {
				return nil, fmt.Errorf("compile exclusion regexp %q: %w", rule.Value, err)
			}
			result.expressions = append(result.expressions, expression)
		}
	}
	return result, nil
}

// covers reports whether item has to be dropped from the direct list. A
// "domain:" rule also covers the subdomains it would match, so excluding
// mail.example.ru removes smtp.mail.example.ru as well.
func (e *excluder) covers(item *routercommon.Domain) bool {
	for _, attribute := range item.Attribute {
		if e.attributes[strings.ToLower(attribute.Key)] {
			return true
		}
	}
	value := item.Value
	switch item.Type {
	case routercommon.Domain_RootDomain, routercommon.Domain_Full:
		if e.coversRoot(value) {
			return true
		}
		if item.Type == routercommon.Domain_Full && e.full[value] {
			return true
		}
	default:
		if e.roots[value] || e.full[value] {
			return true
		}
	}
	for _, keyword := range e.keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	for _, expression := range e.expressions {
		if expression.MatchString(value) {
			return true
		}
	}
	return false
}

func (e *excluder) coversRoot(value string) bool {
	for {
		if e.roots[value] {
			return true
		}
		_, rest, found := strings.Cut(value, ".")
		if !found {
			return false
		}
		value = rest
	}
}

// normalize removes duplicates and orders the list, so that rebuilding an
// unchanged input produces an unchanged output.
func normalize(domains []*routercommon.Domain) []*routercommon.Domain {
	seen := make(map[string]bool, len(domains))
	result := make([]*routercommon.Domain, 0, len(domains))
	for _, domain := range domains {
		key := domainKey(domain)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, domain)
	}
	slices.SortFunc(result, func(a, b *routercommon.Domain) int {
		if order := cmp.Compare(a.Type, b.Type); order != 0 {
			return order
		}
		return cmp.Compare(a.Value, b.Value)
	})
	return result
}

func domainKey(domain *routercommon.Domain) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d:%s", domain.Type, domain.Value)
	keys := make([]string, 0, len(domain.Attribute))
	for _, attribute := range domain.Attribute {
		keys = append(keys, strings.ToLower(attribute.Key))
	}
	slices.Sort(keys)
	for _, key := range keys {
		builder.WriteString("@")
		builder.WriteString(key)
	}
	return builder.String()
}

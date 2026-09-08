package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

// Config is the build configuration, read from config.json.
type Config struct {
	// SourceRepository is the owner/name of the GitHub repository whose latest
	// release carries the upstream domain list.
	SourceRepository string `json:"source_repository"`
	// SourceAsset is the release asset to download.
	SourceAsset string `json:"source_asset"`
	// VerifyChecksum compares the asset against its "<asset>.sha256sum"
	// sibling in the same release.
	VerifyChecksum bool `json:"verify_checksum"`
	// OutputFile is the geosite.dat file to write.
	OutputFile string `json:"output_file"`
	// Category is the code the collected domains are written under, queried
	// from xray-core as "geosite:<category>".
	Category string `json:"category"`
	// ExcludeCategory, when set, writes everything the exclusion rules matched
	// to the same file as a second category. A .dat file cannot express "this
	// suffix except that domain", so a domain that stays reachable through a
	// broader rule of Category has to be routed by a rule of its own; matching
	// ExcludeCategory before Category is what makes the exclusion effective.
	ExcludeCategory string `json:"exclude_category"`
	// IncludeCategories lists the upstream codes to merge into Category.
	IncludeCategories []string `json:"include_categories"`
	// ExcludeCategories lists upstream codes whose domains are subtracted from
	// Category, on top of Exclude.
	ExcludeCategories []string `json:"exclude_categories"`
	// ExcludeAttributes drops every domain carrying one of these upstream
	// attributes, "ads" for example.
	ExcludeAttributes []string `json:"exclude_attributes"`
	// Include lists extra domain rules to add to Category.
	Include []string `json:"include"`
	// Exclude lists domain rules to subtract from Category. Exclusion wins over
	// every other setting, Include included.
	Exclude []string `json:"exclude"`

	include []*routercommon.Domain
	exclude []*routercommon.Domain
}

func loadConfig(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err = config.parse(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &config, nil
}

func (c *Config) parse() error {
	if c.SourceRepository == "" {
		return fmt.Errorf("missing source_repository")
	}
	if c.SourceAsset == "" {
		return fmt.Errorf("missing source_asset")
	}
	if c.OutputFile == "" {
		return fmt.Errorf("missing output_file")
	}
	if c.Category == "" {
		return fmt.Errorf("missing category")
	}
	if len(c.IncludeCategories) == 0 && len(c.Include) == 0 {
		return fmt.Errorf("missing include_categories")
	}
	if strings.EqualFold(c.Category, c.ExcludeCategory) {
		return fmt.Errorf("category and exclude_category are both %q", c.Category)
	}
	lowercase(c.IncludeCategories)
	lowercase(c.ExcludeCategories)
	lowercase(c.ExcludeAttributes)
	var err error
	if c.include, err = parseDomainRules(c.Include); err != nil {
		return fmt.Errorf("include: %w", err)
	}
	if c.exclude, err = parseDomainRules(c.Exclude); err != nil {
		return fmt.Errorf("exclude: %w", err)
	}
	return nil
}

func lowercase(list []string) {
	for index, item := range list {
		list[index] = strings.ToLower(strings.TrimSpace(item))
	}
}

func parseDomainRules(list []string) ([]*routercommon.Domain, error) {
	rules := make([]*routercommon.Domain, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		rule, err := parseDomainRule(item)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// parseDomainRule reads one entry of the include/exclude lists. The syntax is
// the one xray-core uses in its own routing rules: "domain:example.ru" (the
// domain and its subdomains, also the default for a bare "example.ru"),
// "full:www.example.ru", "keyword:example" and "regexp:^ad\..+\.ru$".
func parseDomainRule(rule string) (*routercommon.Domain, error) {
	prefix, value, found := strings.Cut(rule, ":")
	if !found {
		prefix, value = "domain", rule
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil, fmt.Errorf("empty value in rule %q", rule)
	}
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "domain", "suffix":
		return &routercommon.Domain{Type: routercommon.Domain_RootDomain, Value: value}, nil
	case "full":
		return &routercommon.Domain{Type: routercommon.Domain_Full, Value: value}, nil
	case "keyword":
		return &routercommon.Domain{Type: routercommon.Domain_Plain, Value: value}, nil
	case "regexp", "regex":
		// Keep the original case, a regular expression is not a domain name.
		expression := strings.TrimSpace(rule[len(prefix)+1:])
		if _, err := regexp.Compile(expression); err != nil {
			return nil, fmt.Errorf("invalid regexp in rule %q: %w", rule, err)
		}
		return &routercommon.Domain{Type: routercommon.Domain_Regex, Value: expression}, nil
	default:
		return nil, fmt.Errorf("invalid rule %q, expected one of domain:, full:, keyword:, regexp:", rule)
	}
}

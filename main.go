package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v45/github"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

var githubClient *github.Client

func init() {
	log.SetFlags(0)
	accessToken, loaded := os.LookupEnv("ACCESS_TOKEN")
	if !loaded {
		githubClient = github.NewClient(nil)
		return
	}
	transport := &github.BasicAuthTransport{
		Username: accessToken,
	}
	githubClient = github.NewClient(transport.Client())
}

// fetch returns the release the domain list is taken from: the one named by
// FIXED_RELEASE when that is set, the latest one otherwise.
func fetch(from string) (*github.RepositoryRelease, error) {
	names := strings.SplitN(from, "/", 2)
	if len(names) != 2 {
		return nil, fmt.Errorf("invalid repository %q, expected owner/name", from)
	}
	if fixedRelease := os.Getenv("FIXED_RELEASE"); fixedRelease != "" {
		release, _, err := githubClient.Repositories.GetReleaseByTag(context.Background(), names[0], names[1], fixedRelease)
		return release, err
	}
	release, _, err := githubClient.Repositories.GetLatestRelease(context.Background(), names[0], names[1])
	return release, err
}

func get(downloadURL string) ([]byte, error) {
	log.Println("download", downloadURL)
	response, err := http.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", downloadURL, response.Status)
	}
	return io.ReadAll(response.Body)
}

func asset(release *github.RepositoryRelease, name string) ([]byte, error) {
	for _, item := range release.Assets {
		if item.GetName() == name {
			return get(item.GetBrowserDownloadURL())
		}
	}
	return nil, fmt.Errorf("%s not found in upstream release %s", name, release.GetTagName())
}

func download(release *github.RepositoryRelease, name string, verifyChecksum bool) ([]byte, error) {
	data, err := asset(release, name)
	if err != nil {
		return nil, err
	}
	if !verifyChecksum {
		return data, nil
	}
	remoteChecksum, err := asset(release, name+".sha256sum")
	if err != nil {
		return nil, err
	}
	checksum := sha256.Sum256(data)
	if len(remoteChecksum) < 64 || hex.EncodeToString(checksum[:]) != string(remoteChecksum[:64]) {
		return nil, fmt.Errorf("checksum mismatch on %s", name)
	}
	return data, nil
}

// parse groups the upstream domain list by lowercase code.
func parse(binary []byte) (map[string][]*routercommon.Domain, error) {
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(binary, &list); err != nil {
		return nil, err
	}
	domainMap := make(map[string][]*routercommon.Domain, len(list.Entry))
	for _, entry := range list.Entry {
		domainMap[strings.ToLower(entry.CountryCode)] = entry.Domain
	}
	return domainMap, nil
}

func collect(domainMap map[string][]*routercommon.Domain, codes []string) ([]*routercommon.Domain, error) {
	var domains []*routercommon.Domain
	for _, code := range codes {
		entry, loaded := domainMap[code]
		if !loaded {
			return nil, fmt.Errorf("code %q not found in the upstream domain list", code)
		}
		domains = append(domains, entry...)
	}
	return domains, nil
}

func generate(config *Config) (string, error) {
	release, err := fetch(config.SourceRepository)
	if err != nil {
		return "", err
	}
	binary, err := download(release, config.SourceAsset, config.VerifyChecksum)
	if err != nil {
		return "", err
	}
	domainMap, err := parse(binary)
	if err != nil {
		return "", err
	}
	log.Printf("upstream release %s: %d codes", release.GetTagName(), len(domainMap))

	included, err := collect(domainMap, config.IncludeCategories)
	if err != nil {
		return "", err
	}
	included = append(included, config.include...)
	excluded, err := collect(domainMap, config.ExcludeCategories)
	if err != nil {
		return "", err
	}
	excluded = append(excluded, config.exclude...)

	rules, err := newExcluder(excluded, config.ExcludeAttributes)
	if err != nil {
		return "", err
	}
	candidates := normalize(included)
	kept := make([]*routercommon.Domain, 0, len(candidates))
	for _, domain := range candidates {
		if rules.covers(domain) {
			continue
		}
		kept = append(kept, domain)
	}
	log.Printf("category %s: %d domains, %d dropped by the exclusion rules",
		strings.ToUpper(config.Category), len(kept), len(candidates)-len(kept))

	entries := []*routercommon.GeoSite{{
		CountryCode: strings.ToUpper(config.Category),
		Domain:      kept,
	}}
	if config.ExcludeCategory != "" && len(excluded) > 0 {
		excluded = normalize(excluded)
		log.Printf("category %s: %d domains", strings.ToUpper(config.ExcludeCategory), len(excluded))
		entries = append(entries, &routercommon.GeoSite{
			CountryCode: strings.ToUpper(config.ExcludeCategory),
			Domain:      excluded,
		})
	}
	if err = write(config.OutputFile, entries); err != nil {
		return "", err
	}
	return release.GetTagName(), nil
}

func write(path string, entries []*routercommon.GeoSite) error {
	binary, err := proto.Marshal(&routercommon.GeoSiteList{Entry: entries})
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	log.Println("write", absolute)
	return os.WriteFile(path, binary, 0o644)
}

// setActionOutput publishes a step output when running under GitHub Actions.
func setActionOutput(name string, content string) error {
	outputPath := os.Getenv("GITHUB_OUTPUT")
	if outputPath == "" {
		return nil
	}
	file, err := os.OpenFile(outputPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "%s=%s\n", name, content)
	return err
}

func main() {
	configPath := os.Getenv("CONFIG")
	if configPath == "" {
		configPath = "config.json"
	}
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}
	tag, err := generate(config)
	if err != nil {
		log.Fatal(err)
	}
	if err = setActionOutput("tag", tag); err != nil {
		log.Fatal(err)
	}
}

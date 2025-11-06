package main

import (
	acmetest "github.com/cert-manager/cert-manager/test/acme"
	"os"
	"testing"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
	fqdn string
)

func TestRunsSuite(t *testing.T) {
	// The manifest path should contain a file named config.json that is a
	// snippet of valid configuration that should be included on the
	// ChallengeRequest passed as part of the test cases.

	fixture := acmetest.NewFixture(&regeryDNSProviderSolver{},
		acmetest.SetResolvedZone(zone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath("testdata/regery"),
		acmetest.SetUseAuthoritative(true),
	)

	//need to uncomment and  RunConformance delete runBasic and runExtended once https://github.com/cert-manager/cert-manager/pull/4835 is merged
	//fixture.RunConformance(t)
	fixture.RunBasic(t)
	fixture.RunExtended(t)

}

package main

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	prowconfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/plugins"

	"github.com/openshift/ci-tools/pkg/api"
	ciopconfig "github.com/openshift/ci-tools/pkg/config"
)

func TestEditProwConfig(t *testing.T) {
	var testCases = []struct {
		name       string
		prowConfig *prowconfig.Config
		config     initConfig
		expected   *prowconfig.Config
	}{
		{
			name: "queries already exist, nothing changes",
			config: initConfig{
				Org:  "org",
				Repo: "repo",
			},
			prowConfig: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"org/repo"},
						}},
					},
				},
			},
			expected: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"org/repo"},
						}},
					},
				},
			},
		},
		{
			name: "repo does not need bugzilla",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Promotes:              true,
				PromotesWithOpenShift: false,
			},
			prowConfig: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"openshift/ci-tools"},
						}},
					},
				},
			},
			expected: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"openshift/ci-tools", "org/repo"},
						}},
					},
				},
			},
		},
		{
			name: "repo needs bugzilla",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Promotes:              true,
				PromotesWithOpenShift: true,
			},
			prowConfig: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"openshift/origin"},
						}},
					},
				},
			},
			expected: &prowconfig.Config{
				ProwConfig: prowconfig.ProwConfig{
					Tide: prowconfig.Tide{
						Queries: prowconfig.TideQueries{{
							Repos: []string{"openshift/origin", "org/repo"},
						}},
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			editProwConfig(testCase.prowConfig, testCase.config)
			if actual, expected := testCase.prowConfig, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect edited Prow config: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestEditPluginConfig(t *testing.T) {
	no := false
	var testCases = []struct {
		name         string
		pluginConfig *plugins.Configuration
		config       initConfig
		expected     *plugins.Configuration
	}{
		// TODO: actual approve and LGTM cases once the logic is worked out
		{
			name: "no prior records gets everything added",
			config: initConfig{
				Org:    "org",
				Repo:   "repo",
				Branch: "branch",
			},
			pluginConfig: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{},
				Lgtm:    []plugins.Lgtm{},
			},
			expected: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org/repo":         {"foo", "bar"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
					"org/repo":  {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{{
					Repos:               []string{"org/repo"},
					RequireSelfApproval: &no,
					LgtmActsAsApprove:   false,
				}},
				Lgtm: []plugins.Lgtm{{
					Repos:            []string{"org/repo"},
					ReviewActsAsLgtm: true,
				}},
			},
		},
		{
			name: "org already has plugins configured",
			config: initConfig{
				Org:    "org",
				Repo:   "repo",
				Branch: "branch",
			},
			pluginConfig: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org":              {"other"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{},
				Lgtm:    []plugins.Lgtm{},
			},
			expected: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org":              {"other"},
					"org/repo":         {"bar"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
					"org/repo":  {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{{
					Repos:               []string{"org/repo"},
					RequireSelfApproval: &no,
					LgtmActsAsApprove:   false,
				}},
				Lgtm: []plugins.Lgtm{{
					Repos:            []string{"org/repo"},
					ReviewActsAsLgtm: true,
				}},
			},
		},
		{
			name: "org and repo already have plugins configured",
			config: initConfig{
				Org:    "org",
				Repo:   "repo",
				Branch: "branch",
			},
			pluginConfig: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org":              {"other"},
					"org/repo":         {"something"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{},
				Lgtm:    []plugins.Lgtm{},
			},
			expected: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org":              {"other"},
					"org/repo":         {"something"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
					"org/repo":  {{Endpoint: "oops"}},
				},
				Approve: []plugins.Approve{{
					Repos:               []string{"org/repo"},
					RequireSelfApproval: &no,
					LgtmActsAsApprove:   false,
				}},
				Lgtm: []plugins.Lgtm{{
					Repos:            []string{"org/repo"},
					ReviewActsAsLgtm: true,
				}},
			},
		},
		{
			name: "org already has external plugins",
			config: initConfig{
				Org:    "org",
				Repo:   "repo",
				Branch: "branch",
			},
			pluginConfig: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
					"org":       {{Endpoint: "woops"}},
				},
				Approve: []plugins.Approve{},
				Lgtm:    []plugins.Lgtm{},
			},
			expected: &plugins.Configuration{
				Plugins: map[string][]string{
					"openshift":        {"foo"},
					"openshift/origin": {"bar"},
					"org/repo":         {"foo", "bar"},
				},
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					"openshift": {{Endpoint: "oops"}},
					"org":       {{Endpoint: "woops"}},
				},
				Approve: []plugins.Approve{{
					Repos:               []string{"org/repo"},
					RequireSelfApproval: &no,
					LgtmActsAsApprove:   false,
				}},
				Lgtm: []plugins.Lgtm{{
					Repos:            []string{"org/repo"},
					ReviewActsAsLgtm: true,
				}},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			editPluginConfig(testCase.pluginConfig, testCase.config)
			if actual, expected := testCase.pluginConfig, testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect edited Prow plugin config: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func strP(str string) *string {
	return &str
}

func TestGenerateCIOperatorConfig(t *testing.T) {
	var testCases = []struct {
		name         string
		originConfig *api.PromotionConfiguration
		config       initConfig
		expected     ciopconfig.DataWithInfo
	}{
		{
			name: "minimal options",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Branch:                "branch",
				CanonicalGoRepository: "sometimes.com",
				GoVersion:             "1",
				BuildCommands:         "make",
				TestBuildCommands:     "make tests",
			},
			originConfig: &api.PromotionConfiguration{
				Namespace: "promote",
				Name:      "version",
			},
			expected: ciopconfig.DataWithInfo{
				Configuration: api.ReleaseBuildConfiguration{
					InputConfiguration: api.InputConfiguration{
						BuildRootImage: &api.BuildRootImageConfiguration{
							ImageStreamTagReference: &api.ImageStreamTagReference{
								Namespace: "openshift",
								Name:      "release",
								Tag:       "golang-1",
							},
						},
					},
					BinaryBuildCommands:     "make",
					TestBinaryBuildCommands: "make tests",
					CanonicalGoRepository:   strP("sometimes.com"),
					Tests:                   []api.TestStepConfiguration{},
					Resources: map[string]api.ResourceRequirements{"*": {
						Limits:   map[string]string{"memory": "4Gi"},
						Requests: map[string]string{"memory": "200Mi", "cpu": "100m"},
					}},
				},
				Info: ciopconfig.Info{
					Org:    "org",
					Repo:   "repo",
					Branch: "branch",
				},
			},
		},
		{
			name: "promoting into the ecosystem",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Branch:                "branch",
				CanonicalGoRepository: "sometimes.com",
				GoVersion:             "1",
				Promotes:              true,
			},
			originConfig: &api.PromotionConfiguration{
				Namespace: "promote",
				Name:      "version",
			},
			expected: ciopconfig.DataWithInfo{
				Configuration: api.ReleaseBuildConfiguration{
					PromotionConfiguration: &api.PromotionConfiguration{
						Namespace: "promote",
						Name:      "version",
					},
					InputConfiguration: api.InputConfiguration{
						ReleaseTagConfiguration: &api.ReleaseTagConfiguration{
							Namespace: "promote",
							Name:      "version",
						},
						BuildRootImage: &api.BuildRootImageConfiguration{
							ImageStreamTagReference: &api.ImageStreamTagReference{
								Namespace: "openshift",
								Name:      "release",
								Tag:       "golang-1",
							},
						},
					},
					CanonicalGoRepository: strP("sometimes.com"),
					Tests:                 []api.TestStepConfiguration{},
					Resources: map[string]api.ResourceRequirements{"*": {
						Limits:   map[string]string{"memory": "4Gi"},
						Requests: map[string]string{"memory": "200Mi", "cpu": "100m"},
					}},
				},
				Info: ciopconfig.Info{
					Org:    "org",
					Repo:   "repo",
					Branch: "branch",
				},
			},
		},
		{
			name: "releasing with openshift adds e2e",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Branch:                "branch",
				CanonicalGoRepository: "sometimes.com",
				GoVersion:             "1",
				Promotes:              true,
				PromotesWithOpenShift: true,
			},
			originConfig: &api.PromotionConfiguration{
				Namespace: "promote",
				Name:      "version",
			},
			expected: ciopconfig.DataWithInfo{
				Configuration: api.ReleaseBuildConfiguration{
					PromotionConfiguration: &api.PromotionConfiguration{
						Namespace: "promote",
						Name:      "version",
					},
					InputConfiguration: api.InputConfiguration{
						ReleaseTagConfiguration: &api.ReleaseTagConfiguration{
							Namespace: "promote",
							Name:      "version",
						},
						BuildRootImage: &api.BuildRootImageConfiguration{
							ImageStreamTagReference: &api.ImageStreamTagReference{
								Namespace: "openshift",
								Name:      "release",
								Tag:       "golang-1",
							},
						},
					},
					CanonicalGoRepository: strP("sometimes.com"),
					Tests: []api.TestStepConfiguration{{
						As:       "e2e-aws",
						Commands: "TEST_SUITE=openshift/conformance/parallel run-tests",
						OpenshiftInstallerClusterTestConfiguration: &api.OpenshiftInstallerClusterTestConfiguration{
							ClusterTestConfiguration: api.ClusterTestConfiguration{
								ClusterProfile: api.ClusterProfileAWS,
							},
						},
					}},
					Resources: map[string]api.ResourceRequirements{"*": {
						Limits:   map[string]string{"memory": "4Gi"},
						Requests: map[string]string{"memory": "200Mi", "cpu": "100m"},
					}},
				},
				Info: ciopconfig.Info{
					Org:    "org",
					Repo:   "repo",
					Branch: "branch",
				},
			},
		},
		{
			name: "special base images required",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Branch:                "branch",
				CanonicalGoRepository: "sometimes.com",
				GoVersion:             "1",
				NeedsOS:               true,
				NeedsBase:             true,
			},
			originConfig: &api.PromotionConfiguration{
				Namespace: "promote",
				Name:      "version",
			},
			expected: ciopconfig.DataWithInfo{
				Configuration: api.ReleaseBuildConfiguration{
					InputConfiguration: api.InputConfiguration{
						BuildRootImage: &api.BuildRootImageConfiguration{
							ImageStreamTagReference: &api.ImageStreamTagReference{
								Namespace: "openshift",
								Name:      "release",
								Tag:       "golang-1",
							},
						},
						BaseImages: map[string]api.ImageStreamTagReference{
							"base": {
								Namespace: "ocp",
								Name:      "4.3",
								Tag:       "base",
							},
							"os": {
								Namespace: "openshift",
								Name:      "centos",
								Tag:       "7",
							},
						},
					},
					CanonicalGoRepository: strP("sometimes.com"),
					Tests:                 []api.TestStepConfiguration{},
					Resources: map[string]api.ResourceRequirements{"*": {
						Limits:   map[string]string{"memory": "4Gi"},
						Requests: map[string]string{"memory": "200Mi", "cpu": "100m"},
					}},
				},
				Info: ciopconfig.Info{
					Org:    "org",
					Repo:   "repo",
					Branch: "branch",
				},
			},
		},
		{
			name: "tests configured",
			config: initConfig{
				Org:                   "org",
				Repo:                  "repo",
				Branch:                "branch",
				CanonicalGoRepository: "sometimes.com",
				GoVersion:             "1",
				Tests: []test{
					{As: "unit", Command: "make test-unit", From: "src"},
					{As: "cmd", Command: "make test-cmd", From: "bin"},
				},
				CustomE2E: []e2eTest{
					{As: "operator-e2e", Command: "make e2e", Profile: "aws"},
					{As: "operator-e2e-gcp", Command: "make e2e", Profile: "gcp"},
				},
			},
			originConfig: &api.PromotionConfiguration{
				Namespace: "promote",
				Name:      "version",
			},
			expected: ciopconfig.DataWithInfo{
				Configuration: api.ReleaseBuildConfiguration{
					InputConfiguration: api.InputConfiguration{
						BuildRootImage: &api.BuildRootImageConfiguration{
							ImageStreamTagReference: &api.ImageStreamTagReference{
								Namespace: "openshift",
								Name:      "release",
								Tag:       "golang-1",
							},
						},
					},
					CanonicalGoRepository: strP("sometimes.com"),
					Tests: []api.TestStepConfiguration{
						{
							As:       "unit",
							Commands: "make test-unit",
							ContainerTestConfiguration: &api.ContainerTestConfiguration{
								From: "src",
							},
						},
						{
							As:       "cmd",
							Commands: "make test-cmd",
							ContainerTestConfiguration: &api.ContainerTestConfiguration{
								From: "bin",
							},
						},
						{
							As:       "operator-e2e",
							Commands: "make e2e",
							OpenshiftInstallerSrcClusterTestConfiguration: &api.OpenshiftInstallerSrcClusterTestConfiguration{
								ClusterTestConfiguration: api.ClusterTestConfiguration{
									ClusterProfile: "aws",
								},
							},
						},
						{
							As:       "operator-e2e-gcp",
							Commands: "make e2e",
							OpenshiftInstallerSrcClusterTestConfiguration: &api.OpenshiftInstallerSrcClusterTestConfiguration{
								ClusterTestConfiguration: api.ClusterTestConfiguration{
									ClusterProfile: "gcp",
								},
							},
						},
					},
					Resources: map[string]api.ResourceRequirements{"*": {
						Limits:   map[string]string{"memory": "4Gi"},
						Requests: map[string]string{"memory": "200Mi", "cpu": "100m"},
					}},
				},
				Info: ciopconfig.Info{
					Org:    "org",
					Repo:   "repo",
					Branch: "branch",
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := generateCIOperatorConfig(testCase.config, testCase.originConfig), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect generated CI Operator config: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

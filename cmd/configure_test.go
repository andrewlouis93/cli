package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/exercism/cli/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestConfigure(t *testing.T) {
	// Make sure we put back the config env var.
	cfgHomeKey := "EXERCISM_CONFIG_HOME"
	cfgHome := os.Getenv(cfgHomeKey)
	defer os.Setenv(cfgHomeKey, cfgHome)

	// Make sure we put back the real command-line arguments.
	osArgs := os.Args
	defer func() {
		os.Args = osArgs
	}()

	// Set up a bogus root command.
	fakeCmd := &cobra.Command{}
	// Add the real configureCmd to it.
	fakeCmd.AddCommand(configureCmd)

	tests := []struct {
		args           []string
		existingUsrCfg *config.UserConfig
		expectedUsrCfg *config.UserConfig
		existingAPICfg *config.APIConfig
		expectedAPICfg *config.APIConfig
	}{
		{
			// It writes the flags when there is no config file.
			args:           []string{"fakeapp", "configure", "--token", "a", "--workspace", "/a", "--api", "http://example.com"},
			existingUsrCfg: nil,
			expectedUsrCfg: &config.UserConfig{Token: "a", Workspace: "/a"},
			existingAPICfg: nil,
			expectedAPICfg: &config.APIConfig{BaseURL: "http://example.com"},
		},
		{
			// It overwrites the flags in the config file.
			args:           []string{"fakeapp", "configure", "--token", "b", "--workspace", "/b", "--api", "http://example.com/v2"},
			existingUsrCfg: &config.UserConfig{Token: "token-b", Workspace: "/workspace-b"},
			expectedUsrCfg: &config.UserConfig{Token: "b", Workspace: "/b"},
			existingAPICfg: &config.APIConfig{BaseURL: "http://example.com/v1"},
			expectedAPICfg: &config.APIConfig{BaseURL: "http://example.com/v2"},
		},
		{
			args: []string{"fakeapp", "configure", "--token", "c"},
			// It overwrites the flags that are passed without losing the ones that are not.
			existingUsrCfg: &config.UserConfig{Token: "token-c", Workspace: "/workspace-c"},
			expectedUsrCfg: &config.UserConfig{Token: "c", Workspace: "/workspace-c"},
			// It gets the default API base URL.
			existingAPICfg: &config.APIConfig{},
			expectedAPICfg: &config.APIConfig{BaseURL: "https://api.exercism.com/v1"},
		},
	}

	for i, test := range tests {
		// Create a fake config dir.
		dir, err := ioutil.TempDir("", fmt.Sprintf("user-config-%d", i))
		assert.NoError(t, err)
		defer os.RemoveAll(dir)

		// Override the environment to use the fake config dir.
		os.Setenv(cfgHomeKey, dir)

		if test.existingUsrCfg != nil {
			// Write a fake config.
			cfg := config.NewEmptyUserConfig()
			cfg.Token = test.existingUsrCfg.Token
			cfg.Workspace = test.existingUsrCfg.Workspace
			err = cfg.Write()
			assert.NoError(t, err)
		}

		// Fake out the command-line arguments with the correct subcommand.
		os.Args = test.args

		// Re-initialize the command so it picks up the fake environment.
		configureCmd.ResetFlags()
		// Rerun the config initialization so that the flags get bound properly.
		initConfigureCfg()

		// Finally. Execute the configure command.
		fakeCmd.Execute()

		// Now let's get a new config and see that it got written properly.
		usrCfg, err := config.NewUserConfig()
		assert.NoError(t, err)

		assert.Equal(t, test.expectedUsrCfg.Token, usrCfg.Token)
		assert.Equal(t, test.expectedUsrCfg.Workspace, usrCfg.Workspace)

		apiCfg, err := config.NewAPIConfig()
		assert.NoError(t, err)

		assert.Equal(t, test.expectedAPICfg.BaseURL, apiCfg.BaseURL)
	}
}
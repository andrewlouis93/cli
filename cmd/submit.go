package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/exercism/cli/api"
	"github.com/exercism/cli/config"
	"github.com/exercism/cli/workspace"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// submitCmd lets people upload a solution to the website.
var submitCmd = &cobra.Command{
	Use:     "submit",
	Aliases: []string{"s"},
	Short:   "Submit your solution to an exercise.",
	Long: `Submit your solution to an Exercism exercise.

	Call the command with the list of files you want to submit.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.NewConfig()

		usrCfg := viper.New()
		usrCfg.AddConfigPath(cfg.Dir)
		usrCfg.SetConfigName("user")
		usrCfg.SetConfigType("json")
		// Ignore error. If the file doesn't exist, that is fine.
		_ = usrCfg.ReadInConfig()
		cfg.UserViperConfig = usrCfg

		v := viper.New()
		v.AddConfigPath(cfg.Dir)
		v.SetConfigName("cli")
		v.SetConfigType("json")
		// Ignore error. If the file doesn't exist, that is fine.
		_ = v.ReadInConfig()

		return runSubmit(cfg, cmd.Flags(), args)
	},
}

func runSubmit(cfg config.Config, flags *pflag.FlagSet, args []string) error {
	usrCfg := cfg.UserViperConfig

	if usrCfg.GetString("token") == "" {
		return fmt.Errorf(msgWelcomePleaseConfigure, config.SettingsURL(usrCfg.GetString("apibaseurl")), BinaryName)
	}

	if usrCfg.GetString("workspace") == "" {
		return fmt.Errorf(msgRerunConfigure, BinaryName)
	}

	for i, arg := range args {
		var err error
		arg, err = filepath.Abs(arg)
		if err != nil {
			return err
		}

		info, err := os.Lstat(arg)
		if err != nil {
			if os.IsNotExist(err) {
				msg := `

    The file you are trying to submit cannot be found.

        %s

		`
				return fmt.Errorf(msg, arg)
			}
			return err
		}
		if info.IsDir() {
			msg := `

    You are submitting a directory, which is not currently supported.

        %s

    Please change into the directory and provide the path to the file(s) you wish to submit

        %s submit FILENAME

			`
			return fmt.Errorf(msg, arg, BinaryName)
		}

		src, err := filepath.EvalSymlinks(arg)
		if err != nil {
			return err
		}
		args[i] = src
	}

	ws, err := workspace.New(usrCfg.GetString("workspace"))
	if err != nil {
		return err
	}

	var exerciseDir string
	for _, arg := range args {
		dir, err := ws.SolutionDir(arg)
		if err != nil {
			if workspace.IsMissingMetadata(err) {
				return errors.New(msgMissingMetadata)
			}
			return err
		}
		if exerciseDir != "" && dir != exerciseDir {
			msg := `

    You are submitting files belonging to different solutions.
    Please submit the files for one solution at a time.

		`
			return errors.New(msg)
		}
		exerciseDir = dir
	}

	exercise := workspace.NewExerciseFromDir(exerciseDir)

	solution, err := workspace.NewSolution(exerciseDir)
	if err != nil {
		return err
	}

	if !solution.IsRequester {
		// TODO: add test
		msg := `

    The solution you are submitting is not connected to your account.
    Please re-download the exercise to make sure it has the data it needs.

        %s download --exercise=%s --track=%s

		`
		return fmt.Errorf(msg, BinaryName, solution.Exercise, solution.Track)
	}

	exercise.Documents = make([]workspace.Document, 0, len(args))
	for _, file := range args {
		// Don't submit empty files
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if info.Size() == 0 {

			msg := `

    WARNING: Skipping empty file
             %s

		`
			fmt.Fprintf(Err, msg, file)
			continue
		}
		doc, err := workspace.NewDocument(exercise.Filepath(), file)
		if err != nil {
			return err
		}
		exercise.Documents = append(exercise.Documents, doc)
	}

	if len(exercise.Documents) == 0 {
		msg := `

    No files found to submit.

		`
		return errors.New(msg)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for _, doc := range exercise.Documents {
		file, err := os.Open(doc.Filepath())
		if err != nil {
			return err
		}
		defer file.Close()

		part, err := writer.CreateFormFile("files[]", doc.Path())
		if err != nil {
			return err
		}
		_, err = io.Copy(part, file)
		if err != nil {
			return err
		}
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	client, err := api.NewClient(usrCfg.GetString("token"), usrCfg.GetString("apibaseurl"))
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/solutions/%s", usrCfg.GetString("apibaseurl"), solution.ID)
	req, err := client.NewRequest("PATCH", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bb := &bytes.Buffer{}
	_, err = bb.ReadFrom(resp.Body)
	if err != nil {
		return err
	}

	msg := `

    Your solution has been submitted successfully.
    %s
`
	suffix := "View it at:\n\n    "
	if solution.AutoApprove {
		suffix = "You can complete the exercise and unlock the next core exercise at:\n"
	}
	fmt.Fprintf(Err, msg, suffix)
	fmt.Fprintf(Out, "    %s\n\n", solution.URL)
	return nil
}

func init() {
	RootCmd.AddCommand(submitCmd)
}

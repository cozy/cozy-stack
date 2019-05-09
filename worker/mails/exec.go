package mails

import (
	"bytes"
	"io"
	"os"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/worker/exec"
)

func execMjml(ctx *job.WorkerContext, template []byte) ([]byte, error) {
	log := ctx.Logger()

	workDir, err := prepareWorkDir()
	if err != nil {
		log.Errorf("PrepareWorkDir: %s", err)
		return nil, err
	}

	cmdStr, env := prepareCmdEnv(ctx)
	cmd := exec.CreateCmd(cmdStr, workDir)
	cmd.Env = env

	// Send the template on cozy-mjml stdin
	cmd.Stdin = bytes.NewReader(template)

	// Log out all things printed in stderr
	var stderrBuf bytes.Buffer
	cmd.Stderr = utils.LimitWriterDiscard(&stderrBuf, 256*1024)

	out, err := cmd.Output()
	if stderrBuf.Len() > 0 {
		log.Error("Stderr: ", stderrBuf.String())
	}
	if err != nil {
		log.Errorf("Run: %s", err)
		return nil, err
	}

	return out, nil
}

func prepareWorkDir() (string, error) {
	osFS := afero.NewOsFs()
	workDir, err := afero.TempDir(osFS, "", "mjml")
	if err != nil {
		return "", err
	}
	workFS := afero.NewBasePathFs(osFS, workDir)
	dst, err := workFS.OpenFile("index.js", os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return "", err
	}
	f, err := fs.Open("/js/cozy-mjml.js")
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(dst, f)
	if err = dst.Close(); err != nil {
		return "", err
	}
	return workDir, err
}

func prepareCmdEnv(ctx *job.WorkerContext) (string, []string) {
	cmd := config.GetConfig().Konnectors.Cmd
	env := []string{
		"COZY_URL=" + ctx.Instance.PageURL("/", nil),
		"COZY_LANGUAGE=node",
		"COZY_LOCALE=" + ctx.Instance.Locale,
		"COZY_TIME_LIMIT=60",
		"COZY_JOB_ID=" + ctx.ID(),
	}
	return cmd, env
}

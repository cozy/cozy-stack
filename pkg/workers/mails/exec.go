package mails

import (
	"bytes"
	"io"
	"os"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/workers/exec"
)

func execMjml(ctx *jobs.WorkerContext, template []byte) ([]byte, error) {
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

	if err = cmd.Run(); err != nil {
		log.Errorf("Run: %s", err)
		return nil, err
	}
	if stderrBuf.Len() > 0 {
		log.Error("Stderr: ", stderrBuf.String())
	}
	out, err := cmd.Output()
	if err != nil {
		log.Errorf("Output: %s", err)
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
	io.Copy(dst, f)
	if err = dst.Close(); err != nil {
		return "", err
	}
	return workDir, err
}

func prepareCmdEnv(ctx *jobs.WorkerContext) (string, []string) {
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

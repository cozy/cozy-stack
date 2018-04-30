package move

import (
	"fmt"
	"io"
	"io/ioutil"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "export",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      10 * 60 * time.Second,
		WorkerFunc:   Worker,
	})
}

type Options struct {
	IncludeFiles  string
	NoCompression bool
	Output        io.Writer
}

func Worker(c *jobs.WorkerContext) error {
	var opts Options
	if err := c.UnmarshalMessage(&opts); err != nil {
		return err
	}

	f, err := ioutil.TempFile("", "cozy-test")
	if err != nil {
		return err
	}

	opts.Output = f

	i, err := instance.Get(c.Domain())
	if err != nil {
		return err
	}

	fmt.Println(">>>>>>>>", f.Name())
	err = Export(i, opts)
	if err != nil {
		return err
	}

	{
		// 	link := fmt.Sprintf("http://%s%s%s", domain, c.Path(), filename)
		// 	mail := mails.Options{
		// 		Mode:           mails.ModeNoReply,
		// 		TemplateName:   "archiver",
		// 		TemplateValues: map[string]string{"ArchiveLink": link},
		// 	}

		// 	msg, err := jobs.NewMessage(&mail)
		// 	if err != nil {
		// 		return err
		// 	}

		// 	broker := jobs.System()
		// 	_, err = broker.PushJob(&jobs.JobRequest{
		// 		Domain:     instance.Domain,
		// 		WorkerType: "sendmail",
		// 		Message:    msg,
		// 	})
		// 	if err != nil {
		// 		return err
		// 	}
	}

	return nil
}

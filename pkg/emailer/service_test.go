package emailer

import (
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEmailerImplems(t *testing.T) {
	assert.Implements(t, (*Emailer)(nil), new(EmailerService))
	assert.Implements(t, (*Emailer)(nil), new(Mock))
}

func Test_Emailer_success(t *testing.T) {
	brokerMock := job.NewBrokerMock(t)

	emailer := NewEmailerService(brokerMock)

	inst := instance.Instance{}

	brokerMock.On("PushJob", &inst, mock.MatchedBy(func(req *job.JobRequest) bool {
		assert.Equal(t, "sendmail", req.WorkerType)
		assert.JSONEq(t, `{
    "mode": "noreply",
    "template_name": "some-template.html",
    "template_values": {
      "foo": "bar",
      "life": 42
    }
  }`, string(req.Message))
		return true
	})).Return(nil, nil).Once()

	err := emailer.SendEmail(&inst, &SendEmailCmd{
		TemplateName: "some-template.html",
		TemplateValues: map[string]interface{}{
			"foo":  "bar",
			"life": 42,
		},
	})

	assert.NoError(t, err)
}

func Test_Email_job_push_error(t *testing.T) {
	brokerMock := job.NewBrokerMock(t)

	emailer := NewEmailerService(brokerMock)

	inst := instance.Instance{}

	brokerMock.On("PushJob", &inst, mock.Anything).
		Return(nil, fmt.Errorf("some-error")).Once()

	err := emailer.SendEmail(&inst, &SendEmailCmd{
		TemplateName: "some-template.html",
		TemplateValues: map[string]interface{}{
			"foo":  "bar",
			"life": 42,
		},
	})

	assert.EqualError(t, err, "some-error")
}

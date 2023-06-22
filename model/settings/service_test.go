package settings

import (
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/token"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/emailer"
	"github.com/stretchr/testify/assert"
)

func TestServiceImplems(t *testing.T) {
	assert.Implements(t, (*Service)(nil), new(SettingsService))
	assert.Implements(t, (*Service)(nil), new(Mock))
}

func Test_StartEmailUpdate_success(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	cmd := &UpdateEmailCmd{
		Passphrase: []byte("some-pass"),
		Email:      "some@email.com",
	}

	instSvc.On("CheckPassphrase", &inst, cmd.Passphrase).Return(nil).Once()

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{"public_name": "Jane Doe"},
	}, nil).Twice()

	tokenSvc.On("GenerateAndSave", &inst, token.EmailUpdate, "some@email.com", TokenExpiration).
		Return("some-token", nil).Once()

	storage.On("setInstanceSettings", &inst, &couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name":   "Jane Doe",
			"pending_email": "some@email.com",
		},
	}).Return(nil).Once()

	emailerSvc.On("SendEmail", &inst, &emailer.SendEmailCmd{
		TemplateName: "update_email",
		TemplateValues: map[string]interface{}{
			"PublicName":      "Jane Doe",
			"EmailUpdateLink": "http://foo.mycozy.cloud/settings/email/confirm?token=some-token",
		},
	}).Return(nil).Once()

	err := svc.StartEmailUpdate(&inst, cmd)
	assert.NoError(t, err)
}

func Test_StartEmailUpdate_with_an_invalid_password(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	cmd := &UpdateEmailCmd{
		Passphrase: []byte("some-pass"),
		Email:      "some@email.com",
	}

	instSvc.On("CheckPassphrase", &inst, cmd.Passphrase).Return(instance.ErrInvalidPassphrase).Once()

	err := svc.StartEmailUpdate(&inst, cmd)
	assert.ErrorIs(t, err, instance.ErrInvalidPassphrase)
}

func Test_StartEmailUpdate_with_a_missing_public_name(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	cmd := &UpdateEmailCmd{
		Passphrase: []byte("some-pass"),
		Email:      "some@email.com",
	}

	instSvc.On("CheckPassphrase", &inst, cmd.Passphrase).Return(nil).Once()

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		// No field "public_name"
		M: map[string]interface{}{},
	}, nil).Twice()

	tokenSvc.On("GenerateAndSave", &inst, token.EmailUpdate, "some@email.com", TokenExpiration).
		Return("some-token", nil).Once()

	storage.On("setInstanceSettings", &inst, &couchdb.JSONDoc{
		M: map[string]interface{}{
			// There is no public name
			"pending_email": "some@email.com",
		},
	}).Return(nil).Once()

	emailerSvc.On("SendEmail", &inst, &emailer.SendEmailCmd{
		TemplateName: "update_email",
		TemplateValues: map[string]interface{}{
			"PublicName":      "foo", // Change here
			"EmailUpdateLink": "http://foo.mycozy.cloud/settings/email/confirm?token=some-token",
		},
	}).Return(nil).Once()

	err := svc.StartEmailUpdate(&inst, cmd)
	assert.NoError(t, err)
}

func TestConfirmEmailUpdate_success(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name":   "Jane Doe",
			"email":         "foo@bar.baz",
			"pending_email": "some@email.com",
		},
	}, nil).Once()

	tokenSvc.On("Validate", &inst, token.EmailUpdate, "some@email.com", "some-token").
		Return(nil).Once()

	storage.On("setInstanceSettings", &inst, &couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name": "Jane Doe",
			"email":       "some@email.com",
		},
	}).Return(nil).Once()

	err := svc.ConfirmEmailUpdate(&inst, "some-token")
	assert.NoError(t, err)
}

func TestConfirmEmailUpdate_with_an_invalid_token(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name":   "Jane Doe",
			"email":         "foo@bar.baz",
			"pending_email": "some@email.com",
		},
	}, nil).Once()

	tokenSvc.On("Validate", &inst, token.EmailUpdate, "some@email.com", "some-invalid-token").
		Return(token.ErrInvalidToken).Once()

	err := svc.ConfirmEmailUpdate(&inst, "some-invalid-token")
	assert.ErrorIs(t, err, token.ErrInvalidToken)
}

func TestConfirmEmailUpdate_without_a_pending_email(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name": "Jane Doe",
			"email":       "foo@bar.baz",
			// There is no pending_email
		},
	}, nil).Once()

	err := svc.ConfirmEmailUpdate(&inst, "some-token")
	assert.ErrorIs(t, err, ErrNoPendingEmail)
}

func Test_CancelEmailUpdate_success(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name":   "Jane Doe",
			"email":         "foo@bar.baz",
			"pending_email": "some@email.com",
		},
	}, nil).Once()

	storage.On("setInstanceSettings", &inst, &couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name": "Jane Doe",
			"email":       "foo@bar.baz",
		},
	}).Return(nil).Once()

	err := svc.CancelEmailUpdate(&inst)
	assert.NoError(t, err)
}

func Test_CancelEmailUpdate_without_pending_email(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name": "Jane Doe",
			"email":       "foo@bar.baz",
		},
	}, nil).Once()

	err := svc.CancelEmailUpdate(&inst)
	assert.NoError(t, err)
}

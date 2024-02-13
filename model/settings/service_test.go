package settings

import (
	"errors"
	"testing"

	"github.com/cozy/cozy-stack/model/cloudery"
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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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

	emailerSvc.On("SendPendingEmail", &inst, &emailer.TransactionalEmailCmd{
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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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

	emailerSvc.On("SendPendingEmail", &inst, &emailer.TransactionalEmailCmd{
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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
		Locale: "fr/FR",
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
			"public_name":   "Jane Doe",
			"email":         "some@email.com",
			"pending_email": nil,
		},
	}).Return(nil).Once()

	clouderySvc.On("SaveInstance", &inst, &cloudery.SaveCmd{
		Locale:     "fr/FR",
		Email:      "some@email.com",
		PublicName: "Jane Doe",
	}).Return(nil).Once()

	err := svc.ConfirmEmailUpdate(&inst, "some-token")
	assert.NoError(t, err)
}

func TestConfirmEmailUpdate_with_an_invalid_token(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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
			"public_name":   "Jane Doe",
			"email":         "foo@bar.baz",
			"pending_email": nil,
		},
	}).Return(nil).Once()

	err := svc.CancelEmailUpdate(&inst)
	assert.NoError(t, err)
}

func Test_CancelEmailUpdate_without_pending_email(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

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

func Test_ResendEmailUpdate_success(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name":   "Jane Doe",
			"pending_email": "foo.mycozy.cloud",
		},
	}, nil).Twice()

	tokenSvc.On("GenerateAndSave", &inst, token.EmailUpdate, "foo.mycozy.cloud", TokenExpiration).
		Return("some-token", nil).Once()

	emailerSvc.On("SendPendingEmail", &inst, &emailer.TransactionalEmailCmd{
		TemplateName: "update_email",
		TemplateValues: map[string]interface{}{
			"PublicName":      "Jane Doe",
			"EmailUpdateLink": "http://foo.mycozy.cloud/settings/email/confirm?token=some-token",
		},
	}).Return(nil).Once()

	err := svc.ResendEmailUpdate(&inst)
	assert.NoError(t, err)
}

func Test_ResendEmailUpdate_with_no_pending_email(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	storage.On("getInstanceSettings", &inst).Return(&couchdb.JSONDoc{
		M: map[string]interface{}{
			"public_name": "Jane Doe",
			// no pendin_email
		},
	}, nil).Twice()

	err := svc.ResendEmailUpdate(&inst)
	assert.ErrorIs(t, err, ErrNoPendingEmail)
}

func Test_GetExternalTies(t *testing.T) {
	emailerSvc := emailer.NewMock(t)
	instSvc := instance.NewMock(t)
	tokenSvc := token.NewMock(t)
	clouderySvc := cloudery.NewMock(t)
	storage := newStorageMock(t)

	svc := NewService(emailerSvc, instSvc, tokenSvc, clouderySvc, storage)

	inst := instance.Instance{
		Domain: "foo.mycozy.cloud",
	}

	t.Run("with blocking subscription", func(t *testing.T) {
		clouderySvc.On("HasBlockingSubscription", &inst).Return(true, nil).Once()

		ties, err := svc.GetExternalTies(&inst)
		assert.NoError(t, err)
		assert.EqualExportedValues(t, *ties, ExternalTies{HasBlockingSubscription: true})
	})

	t.Run("without blocking subscription", func(t *testing.T) {
		clouderySvc.On("HasBlockingSubscription", &inst).Return(false, nil).Once()

		ties, err := svc.GetExternalTies(&inst)
		assert.NoError(t, err)
		assert.EqualExportedValues(t, *ties, ExternalTies{HasBlockingSubscription: false})
	})

	t.Run("with error from cloudery", func(t *testing.T) {
		unauthorizedError := errors.New("unauthorized")
		clouderySvc.On("HasBlockingSubscription", &inst).Return(false, unauthorizedError).Once()

		ties, err := svc.GetExternalTies(&inst)
		assert.ErrorIs(t, err, unauthorizedError)
		assert.Nil(t, ties)
	})
}

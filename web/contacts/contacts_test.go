package contacts

import (
	"testing"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/assert"
)

func TestContacts(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.com",
		PublicName: "Alice",
	})
	_, token := setup.GetTestClient(consts.Contacts)
	ts := setup.GetTestServer("/contacts", Routes)
	t.Cleanup(ts.Close)

	t.Run("Myself", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Create an empty contact for myself
		e.POST("/contacts/myself").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)

		// Get the contact info about myself
		myself, err := contact.GetMyself(testInstance)
		assert.NoError(t, err)

		// Delete the contacts info about myself
		err = couchdb.DeleteDoc(testInstance, myself)
		assert.NoError(t, err)

		// Create again an empty contact for myself ?
		obj := e.POST("/contacts/myself").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// Check the contact data.
		data := obj.Value("data").Object()
		data.Value("id").String().NotEmpty()
		data.ValueEqual("type", consts.Contacts)

		meta := data.Value("meta").Object()
		meta.Value("rev").String().NotEmpty()

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("fullname", "Alice")
		emails := attrs.Value("email").Array()
		emails.Length().Equal(1)
		email := emails.First().Object()
		email.ValueEqual("address", "alice@example.com")
		email.ValueEqual("primary", true)
	})
}

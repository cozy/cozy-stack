;(function (w, d) {
  const form = d.getElementById('oidc-twofactor-form')
  const trustedTokenInput = d.getElementById('trusted-device-token')

  // Set the trusted device token from the localstorage in the form if it exists
  try {
    const storage = w.localStorage
    const deviceToken = storage.getItem('trusted-device-token') || ''
    trustedTokenInput.value = deviceToken
  } catch (e) {
    // do nothing
  }

  form.submit()
})(window, document)

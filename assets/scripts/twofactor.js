;(function (w, d) {
  if (!w.fetch || !w.Headers) return

  const twofaForm = d.getElementById('two-factor-form')
  const twofaField = d.getElementById('two-factor-field')
  const redirectInput = d.getElementById('redirect')
  const stateInput = d.getElementById('state')
  const confirmInput = d.getElementById('confirm')
  const clientIdInput = d.getElementById('client_id')
  const submitButton = d.getElementById('two-factor-submit')
  const passcodeInput = d.getElementById('two-factor-passcode')
  const tokenInput = d.getElementById('two-factor-token')
  const trustCheckbox = d.getElementById('two-factor-trust-device')
  const longRunCheckbox = d.getElementById('long-run-session')

  const storage = w.localStorage

  const onSubmitTwoFactorCode = function (event) {
    event.preventDefault()
    passcodeInput.setAttribute('disabled', true)
    submitButton.setAttribute('disabled', true)

    const longRun = longRunCheckbox && longRunCheckbox.checked ? '1' : '0'
    const passcode = passcodeInput.value
    const token = tokenInput.value
    const trustDevice = trustCheckbox && trustCheckbox.checked ? '1' : '0'
    const redirect = redirectInput.value + w.location.hash

    const data = new URLSearchParams()
    data.append('two-factor-passcode', passcode)
    data.append('long-run-session', longRun)
    data.append('two-factor-token', token)
    data.append('two-factor-generate-trusted-device-token', trustDevice)
    data.append('redirect', redirect)

    // When 2FA is checked for moving a Cozy to this instance
    if (stateInput) {
      data.append('state', stateInput.value)
    }
    if (clientIdInput) {
      data.append('client_id', clientIdInput.value)
    }

    // When 2FA is checked for confirming authentication
    if (confirmInput && confirmInput.value === 'true') {
      data.append('confirm', true)
    }

    const headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')
    return fetch('/auth/twofactor', {
      method: 'POST',
      headers: headers,
      body: data,
      credentials: 'same-origin',
    })
      .then((response) => {
        return response.json().then(function (body) {
          if (response.status < 400) {
            if (
              storage &&
              typeof body.two_factor_trusted_device_token == 'string'
            ) {
              storage.setItem(
                'trusted-device-token',
                body.two_factor_trusted_device_token
              )
            }
            submitButton.innerHTML = '<span class="icon icon-check"></span>'
            submitButton.classList.add('btn-done')
            w.location = body.redirect
          } else {
            showError(twofaField, body.error)
          }
        })
      })
      .catch(err => showError(twofaField, err))
  }

  twofaForm.addEventListener('submit', onSubmitTwoFactorCode)
})(window, document)

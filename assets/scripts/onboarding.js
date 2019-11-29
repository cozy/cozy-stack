;(function(w, d) {
  if (!w.fetch || !w.Headers) return

  const form = d.getElementById('onboarding-password-form')
  const passphraseInput = d.getElementById('password')
  const hintInput = d.getElementById('hint')
  const submitButton = d.getElementById('onboarding-password-submit')
  const iterationsInput = d.getElementById('onboarding-password-iterations')
  const registerTokenInput = d.getElementById('register-token')

  let errorPanel
  const renewField = d.getElementById('onboarding-password-field')
  const showError = function(error) {
    if (error) {
      error = '' + error
    } else {
      error = 'The Cozy server is unavailable. Do you have network?'
    }

    if (!errorPanel) {
      errorPanel = d.createElement('p')
      errorPanel.classList.add('wizard-errors', 'u-error')
      renewField.insertBefore(errorPanel, renewField.firstChild)
    }

    errorPanel.textContent = error
    submitButton.removeAttribute('disabled')
  }

  form.addEventListener('submit', function(event) {
    if (passphraseInput.classList.contains('is-error')) {
      return
    }
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')

    const hint = hintInput.value
    const salt = form.dataset.salt
    const iterations = parseInt(iterationsInput.value, 10)
    const registerToken = registerTokenInput.value
    let hashed, masterKey

    w.password
      .hash(passphraseInput.value, salt, iterations)
      .then(pass => {
        hashed = pass.hashed
        return w.password.makeEncKey(pass.masterKey)
      })
      .then(key => {
        masterKey = key.cipherString
        return w.password.makeKeyPair(key.key)
      })
      .then(pair => {
        const reqBody =
          'passphrase=' +
          encodeURIComponent(hashed) +
          '&iterations=' +
          encodeURIComponent('' + iterations) +
          '&register_token=' +
          encodeURIComponent(registerToken) +
          '&hint=' +
          encodeURIComponent(hint) +
          '&key=' +
          encodeURIComponent(masterKey) +
          '&public_key=' +
          encodeURIComponent(pair.publicKey) +
          '&private_key=' +
          encodeURIComponent(pair.privateKey)

        return fetch('/settings/passphrase', {
          method: 'POST',
          headers: headers,
          body: reqBody,
          credentials: 'same-origin'
        })
      })
      .then(response => {
        const success = response.status < 400
        response
          .json()
          .then(body => {
            if (success) {
              submitButton.childNodes[1].innerHTML =
                '<svg width="16" height="16"><use xlink:href="#fa-check"/></svg>'
              submitButton.classList.add('c-btn--highlight')
              if (body.redirect) {
                w.location = body.redirect
              }
            } else {
              showError(body.error)
              passphraseInput.classList.add('is-error')
              passphraseInput.select()
            }
          })
          .catch(showError)
      })
      .catch(showError)
  })

  submitButton.removeAttribute('disabled')
})(window, document)

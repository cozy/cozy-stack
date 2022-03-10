;(function (w, d) {
  if (!w.fetch || !w.Headers) return

  const form = d.getElementById('new-pass-form')
  const passField = d.getElementById('password-field')
  const passInput = d.getElementById('password')
  const hintField = d.getElementById('hint-field')
  const hintInput = d.getElementById('hint')
  const strength = d.getElementById('password-strength')
  const submit = form.querySelector('[type=submit]')
  const iterationsInput = d.getElementById('iterations')
  const registerTokenInput = d.getElementById('register-token')
  const resetTokenInput = d.getElementById('reset-token')
  const csrfTokenInput = d.getElementById('csrf_token')

  const querystring = new URLSearchParams(w.location.search)
  const redirection = querystring.get('redirection')

  form.addEventListener('submit', function (event) {
    event.preventDefault()

    const pass = passInput.value
    const hint = hintInput.value
    const salt = form.dataset.salt
    const iterations = parseInt(iterationsInput.value, 10)

    const tooltips = form.querySelectorAll('.invalid-tooltip')
    for (const tooltip of tooltips) {
      tooltip.classList.add('d-none')
    }

    if (hint === pass) {
      w.showError(hintField, form.dataset.hintError)
      return
    }

    if (strength.classList.contains('pass-weak')) {
      w.showError(passField, form.dataset.passError)
      return
    }

    submit.setAttribute('disabled', true)

    let hashed, protectedKey
    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    headers.append('Accept', 'application/json')

    w.password
      .hash(pass, salt, iterations)
      .then((result) => {
        hashed = result.hashed
        return w.password.makeEncKey(result.masterKey)
      })
      .then((key) => {
        protectedKey = key.cipherString
        return w.password.makeKeyPair(key.key)
      })
      .then((pair) => {
        const data = new URLSearchParams()
        data.append('passphrase', hashed)
        if (hint) {
          data.append('hint', hint)
        }
        data.append('iterations', '' + iterations)
        data.append('key', protectedKey)
        data.append('public_key', pair.publicKey)
        data.append('private_key', pair.privateKey)
        if (registerTokenInput) {
          data.append('register_token', registerTokenInput.value)
        }
        if (resetTokenInput) {
          data.append('passphrase_reset_token', resetTokenInput.value)
        }
        if (csrfTokenInput) {
          data.append('csrf_token', csrfTokenInput.value)
        }
        if (redirection) {
          data.append('redirection', redirection)
        }

        return fetch(form.action, {
          method: 'POST',
          headers: headers,
          body: data,
          credentials: 'same-origin',
        })
      })
      .then((response) => {
        return response.json().then((body) => {
          if (response.status < 400) {
            submit.innerHTML = '<span class="icon icon-check"></span>'
            submit.classList.add('btn-done')
            w.location = body.redirect
          } else {
            w.showError(passField, body.error)
          }
        })
      })
      .catch((err) => w.showError(passField, err))
  })

  submit.removeAttribute('disabled')
})(window, document)

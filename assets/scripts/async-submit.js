/* global Headers, fetch */
(function (w) {
  if (!w.fetch || !w.Headers || !w.FormData) return

  const d = window.document
  let form = d.getElementById('login-form')
  const url = form.getAttribute('action')
  const passphraseInput = d.getElementById('password')
  const redirectInput = d.getElementById('redirect')
  const submitButton = d.getElementById('login-submit')
  let errorPanel = form.querySelector('.errors')

  const showError = function (error) {
    error = error.message ? error.message : error

    if (!errorPanel) {
      errorPanel = d.createElement('div')
      errorPanel.classList.add('errors')
      form.appendChild(errorPanel)
    }

    errorPanel.innerHTML = `<p>${error}</p>`
  }

  form.addEventListener('submit', (event) => {
    event.preventDefault()
    submitButton.setAttribute('disabled', true)

    const passphrase = passphraseInput.value
    const redirect = redirectInput.value
    let headers = new Headers()
    headers.append('Content-Type', 'application/x-www-form-urlencoded')
    fetch('/auth/login', {
      method: 'POST',
      headers: headers,
      body: `passphrase=${passphrase}&redirect=${redirect}`,
      redirect: 'manual'
    }).then((response) => {
      // Redirected so passphrase was ok ... I guess.
      const loginSuccess = response.status === 0

      if (loginSuccess) {
        submitButton.innerHTML = '<i class="fa fa-check"></i>'
        submitButton.classList.add('btn-success')
        // Really hackish : The call to POST /auth/login via fetch does not log
        // the user, so if we are here the passphrase seems correct, we submit
        // the form. A second time, yes, but it will actually log the user.
        form.submit()
      } else {
        submitButton.removeAttribute('disabled')
        throw new Error('The credentials you entered are incorrect, please try again.')
      }
    }).catch(showError)
  })

  passphraseInput.focus()
})(window)

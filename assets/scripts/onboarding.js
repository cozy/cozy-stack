;(function(w, d) {
  if (!w.fetch || !w.Headers) return

  let passwordHashed = false
  const form = d.getElementById('onboarding-password-form')
  const passphraseInput = d.getElementById('password')
  const submitButton = d.getElementById('onboarding-password-submit')
  const iterationsInput = d.getElementById('onboarding-password-iterations')
  const keyInput = d.getElementById('onboarding-password-key')

  form.addEventListener('submit', function(event) {
    const salt = form.dataset.salt
    const iterations = parseInt(iterationsInput.value, 10)

    // Pause while hashing the password
    if (!passwordHashed && iterations > 0) {
      event.preventDefault()
      w.password
        .hash(passphraseInput.value, salt, iterations)
        .then(pass => {
          passphraseInput.value = pass.hashed
          passwordHashed = true
          return w.password.makeEncKey(pass.masterKey)
        })
        .then(key => {
          keyInput.value = key
          form.submit()
        })
    }

    submitButton.setAttribute('disabled', true)
  })

  submitButton.removeAttribute('disabled')
})(window, document)

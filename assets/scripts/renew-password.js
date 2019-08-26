;(function(w, d) {
  if (!w.fetch || !w.Headers) return

  let passwordHashed = false
  const form = d.getElementById('renew-passphrase-form')
  const passphraseInput = d.getElementById('password')
  const submitButton = d.getElementById('renew-password-submit')
  const iterationsInput = d.getElementById('renew-password-iterations')

  form.addEventListener('submit', function(event) {
    const salt = form.dataset.salt
    const iterations = parseInt(iterationsInput.value, 10)

    // Pause while hashing the password
    if (!passwordHashed && iterations > 0) {
      event.preventDefault()
      w.password.hash(passphraseInput.value, salt, iterations).then(pass => {
        passphraseInput.value = pass
        passwordHashed = true
        form.submit()
      })
    }

    submitButton.setAttribute('disabled', true)
  })

  submitButton.removeAttribute('disabled')
})(window, document)

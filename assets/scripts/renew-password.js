;(function(w, d) {
  if (!w.fetch || !w.Headers) return

  let passwordHashed = false
  const form = d.getElementById('renew-passphrase-form')
  const passphraseInput = d.getElementById('password')
  const submitButton = d.getElementById('renew-password-submit')
  const iterationsInput = d.getElementById('renew-password-iterations')
  const keyInput = d.getElementById('renew-password-key')
  const publicKeyInput = d.getElementById('renew-password-public-key')
  const privateKeyInput = d.getElementById('renew-password-private-key')

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
          keyInput.value = key.cipherString
          return w.password.makeKeyPair(key.key)
        })
        .then(pair => {
          publicKeyInput.value = pair.publicKey
          privateKeyInput.value = pair.privateKey
          form.submit()
        })
    }

    submitButton.setAttribute('disabled', true)
  })

  submitButton.removeAttribute('disabled')
})(window, document)

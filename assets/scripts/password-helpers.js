;(function(w) {
  // Return given password strength as an object {percentage, label}
  function getStrength(password) {
    if (!password && password !== '') {
      throw new Error('password parameter is missing')
    }
    if (!password.length) {
      return { percentage: 0, label: 'weak' }
    }

    const charsets = [
      // upper
      { regexp: /[A-Z]/g, size: 26 },
      // lower
      { regexp: /[a-z]/g, size: 26 },
      // digit
      { regexp: /[0-9]/g, size: 10 },
      // special
      { regexp: /[!@#$%^&*()_+\-=[\]{};':"\\|,.<>/?]/g, size: 30 }
    ]

    const possibleChars = charsets.reduce(function(possibleChars, charset) {
      if (charset.regexp.test(password)) possibleChars += charset.size
      return possibleChars
    }, 0)

    const passwordStrength =
      Math.log(Math.pow(possibleChars, password.length)) / Math.log(2)

    // levels
    const _at33percent = 50
    const _at66percent = 100
    const _at100percent = 150

    let strengthLabel = ''
    let strengthPercentage = 0

    // between 0% and 33%
    if (passwordStrength <= _at33percent) {
      strengthPercentage = (passwordStrength * 33) / _at33percent
      strengthLabel = 'weak'
    } else if (
      passwordStrength > _at33percent &&
      passwordStrength <= _at66percent
    ) {
      // between 33% and 66%
      strengthPercentage = (passwordStrength * 66) / _at66percent
      strengthLabel = 'moderate'
    } else {
      // passwordStrength > 192
      strengthPercentage = (passwordStrength * 100) / _at100percent
      if (strengthPercentage > 100) strengthPercentage = 100
      strengthLabel = 'strong'
    }

    return { percentage: strengthPercentage, label: strengthLabel }
  }

  function fromUtf8ToBuffer(str) {
    const strUtf8 = unescape(encodeURIComponent(str))
    const arr = new Uint8Array(strUtf8.length)
    for (let i = 0; i < strUtf8.length; i++) {
      arr[i] = strUtf8.charCodeAt(i)
    }
    return arr.buffer
  }

  // Return a promise that resolves to the hash of the master password.
  // TODO use cozy-auth.js from https://github.com/cozy/cozy-keys-lib/tree/init
  // to support Edge
  function hash(password, salt, iterations) {
    const subtle = w.crypto.subtle
    const passwordBuf = fromUtf8ToBuffer(password)
    const saltBuf = fromUtf8ToBuffer(salt)
    const first = {
      name: 'PBKDF2',
      salt: saltBuf,
      iterations: iterations,
      hash: { name: 'SHA-256' }
    }
    const second = {
      name: 'PBKDF2',
      salt: passwordBuf,
      iterations: 1,
      hash: { name: 'SHA-256' }
    }
    return subtle
      .importKey('raw', passwordBuf, { name: 'PBKDF2' }, false, ['deriveBits'])
      .then(material => subtle.deriveBits(first, material, 256))
      .then(key =>
        subtle.importKey('raw', key, { name: 'PBKDF2' }, false, ['deriveBits'])
      )
      .then(material => subtle.deriveBits(second, material, 256))
      .then(hashed => {
        let binary = ''
        const bytes = new Uint8Array(hashed)
        for (let i = 0; i < bytes.byteLength; i++) {
          binary += String.fromCharCode(bytes[i])
        }
        return w.btoa(binary)
      })
  }

  w.password = {
    getStrength: getStrength,
    hash: hash
  }
})(window)

;(function (d) {
  var passwordIsVisible = false
  var passwordInput = d.getElementById('password')
  var passwordVisibilityButton = d.getElementById('password-visibility-button')
  var passwordIconDisplay = d.getElementById('display-icon')
  var passwordIconHide = d.getElementById('hide-icon')

  passwordVisibilityButton.addEventListener('click', function (event) {
    event.preventDefault()
    passwordIsVisible = !passwordIsVisible
    passwordInput.type = passwordIsVisible ? 'text' : 'password'
    passwordInput.setAttribute(
      'autocomplete',
      passwordIsVisible ? 'off' : 'current-password'
    )

    if (passwordIsVisible) {
      passwordIconDisplay.setAttribute('class', '')
      passwordIconHide.setAttribute('class', 'u-hide')
    } else {
      passwordIconDisplay.setAttribute('class', 'u-hide')
      passwordIconHide.setAttribute('class', '')
    }

    passwordVisibilityButton.setAttribute(
      'title',
      passwordIsVisible
        ? passwordVisibilityButton.getAttribute('data-hide')
        : passwordVisibilityButton.getAttribute('data-show')
    )

    passwordInput.focus()
  })
})(window.document)

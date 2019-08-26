;(function(d) {
  var passwordVisibility = false
  var passwordInput = d.getElementById('password')
  var passwordVisibilityButton = d.getElementById('password-visibility-button')
  var passwordIconDisplay = d.getElementById('display-icon')
  var passwordIconHide = d.getElementById('hide-icon')

  passwordVisibilityButton.addEventListener('click', function(event) {
    event.preventDefault()
    passwordVisibility = !passwordVisibility
    passwordInput.type = passwordVisibility ? 'text' : 'password'
    passwordInput.setAttribute(
      'autocomplete',
      passwordVisibility ? 'off' : 'current-password'
    )

    if (passwordVisibility === true) {
      passwordIconDisplay.setAttribute('class', '')
      passwordIconHide.setAttribute('class', 'u-hide')
    } else {
      passwordIconDisplay.setAttribute('class', 'u-hide')
      passwordIconHide.setAttribute('class', '')
    }

    passwordVisibilityButton.setAttribute(
      'title',
      passwordVisibility
        ? passwordVisibilityButton.getAttribute('data-hide')
        : passwordVisibilityButton.getAttribute('data-show')
    )

    passwordInput.focus()
  })
})(window.document)

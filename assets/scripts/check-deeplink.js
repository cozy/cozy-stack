import protocolCheck from './protocol-check.js';

(function (window) {
  let url = window.location.href;
  let queryString = url.split('?')[1]
  let params = new URLSearchParams(queryString)

  let redirect = params.get("redirect_uri")

  protocolCheck(redirect, function () {
    document.getElementById('handle_deeplink').value = 'false';
  })

})(window)

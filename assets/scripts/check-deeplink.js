function _createHiddenIframe(target, uri) {
  const iframe = document.createElement('iframe')
  iframe.src = uri
  iframe.id = 'hiddenIframe'
  iframe.style.display = 'none'
  target.appendChild(iframe)
  return iframe
}

function openUriWithHiddenFrame(uri, failCb) {
  const timeout = setTimeout(function() {
    failCb()
    handler.remove()
  }, 500)

  let iframe = document.getElementById('hiddenIframe')
  if (!iframe) {
    iframe = _createHiddenIframe(document.body, 'about:blank')
  }

  var handler = window.addEventListener('blur', onBlur)
  function onBlur() {
    clearTimeout(timeout)
    handler.remove()
  }

  iframe.contentWindow.location.href = uri
}

function openUriWithTimeoutHack(uri, failCb) {
  const timeout = setTimeout(function() {
    failCb()
    handler.remove()
  }, 500)

  //handle page running in an iframe (blur must be registered with top level window)
  let target = window
  while (target != target.parent) {
    target = target.parent
  }

  var handler = target.addEventListener('blur', onBlur)
  function onBlur() {
    if (handler) {
      clearTimeout(timeout)
      handler.remove()
    }
  }

  window.location = uri
}

function openUriWithMsLaunchUri(uri, failCb) {
  navigator.msLaunchUri(uri, undefined, failCb)
}

function checkBrowser() {
  const isOpera = !!window.opera || navigator.userAgent.indexOf(' OPR/') >= 0
  const ua = navigator.userAgent.toLowerCase()
  const isSafari =
    (~ua.indexOf('safari') && !~ua.indexOf('chrome')) ||
    Object.prototype.toString.call(window.HTMLElement).indexOf('Constructor') >
      0
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) && !window.MSStream
  const isIOS122 = isIOS && (ua.includes('os 12_2') || ua.includes('os 12_3'))
  return {
    isOpera,
    isFirefox: typeof InstallTrigger !== 'undefined',
    isSafari,
    isIOS,
    isIOS122,
    isChrome: !!window.chrome && !isOpera
  }
}

function check(uri, failCb) {
  if (navigator.msLaunchUri) {
    //for IE and Edge in Win 8 and Win 10
    openUriWithMsLaunchUri(uri, failCb)
  } else {
    const browser = checkBrowser()

    if (browser.isChrome || (browser.isIOS && !browser.isIOS122)) {
      openUriWithTimeoutHack(uri, failCb)
    } else if ((browser.isSafari && !browser.isIOS122) || browser.isFirefox) {
      openUriWithHiddenFrame(uri, failCb)
    } else {
      failCb()
    }
  }
}

;(function(window) {
  let params = new URLSearchParams(document.location.search.substring(1))

  const fallbackUri = params.get('fallback_uri')
  const form = document.getElementById('authorizeform')
  form.addEventListener('submit', function(e) {
    /*
      We want to call manually the /authorize route
      in order to get a JSON response containing the deeplink
      to use.
      So we need to create the request
    */
    e.preventDefault()
    const arr = []
    for (var i = 0; i < form.elements.length; i++) {
      const el = form.elements[i]
      arr.push(encodeURIComponent(el.name) + '=' + encodeURIComponent(el.value))
    }
    const bodyString = arr.join('&')
    const action = form.action
    /*
      We check if the client can open the deeplink. If not, we force the
      redirection to the fallbackURI
    */

    fetch(action, {
      method: 'POST',
      body: bodyString,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        Accept: 'application/json'
      }
    })
      .then(function(response) {
        return response.json()
      })
      .then(function(json) {
        check(json.deeplink, function() {
          window.location.href = fallbackUri
        })
      })
  })
})(window)

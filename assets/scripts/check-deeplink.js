function _registerEvent(target, eventType, cb) {
  if (target.addEventListener) {
    target.addEventListener(eventType, cb);
    return {
      remove: function() {
        target.removeEventListener(eventType, cb);
      }
    };
  } else {
    target.attachEvent(eventType, cb);
    return {
      remove: function() {
        target.detachEvent(eventType, cb);
      }
    };
  }
}

function _createHiddenIframe(target, uri) {
  var iframe = document.createElement("iframe");
  iframe.src = uri;
  iframe.id = "hiddenIframe";
  iframe.style.display = "none";
  target.appendChild(iframe);

  return iframe;
}

function openUriWithHiddenFrame(uri, failCb, successCb) {
  var timeout = setTimeout(function() {
    failCb();
    handler.remove();
  }, 1000);

  var iframe = document.querySelector("#hiddenIframe");
  if (!iframe) {
    iframe = _createHiddenIframe(document.body, "about:blank");
  }

  var handler = _registerEvent(window, "blur", onBlur);

  function onBlur() {
    clearTimeout(timeout);
    handler.remove();
    successCb();
  }

  iframe.contentWindow.location.href = uri;
}

function openUriWithTimeoutHack(uri, failCb, successCb) {
  var timeout = setTimeout(function() {
    failCb();
    handler.remove();
  }, 1000);

  //handle page running in an iframe (blur must be registered with top level window)
  var target = window;
  while (target != target.parent) {
    target = target.parent;
  }

  var handler = _registerEvent(target, "blur", onBlur);

  function onBlur() {
    clearTimeout(timeout);
    handler.remove();
    successCb();
  }

  window.location = uri;
}

function openUriUsingFirefox(uri, failCb, successCb) {
  var iframe = document.querySelector("#hiddenIframe");
  if (!iframe) {
    iframe = _createHiddenIframe(document.body, "about:blank");
  }
  try {
    iframe.contentWindow.location.href = uri;
    successCb();
  } catch (e) {
    if (e.name == "NS_ERROR_UNKNOWN_PROTOCOL") {
      failCb();
    }
  }
}

function openUriWithMsLaunchUri(uri, failCb, successCb) {
  navigator.msLaunchUri(uri, successCb, failCb);
}

function checkBrowser() {
  var isOpera = !!window.opera || navigator.userAgent.indexOf(" OPR/") >= 0;
  var ua = navigator.userAgent.toLowerCase();
  return {
    isOpera: isOpera,
    isFirefox: typeof InstallTrigger !== "undefined",
    isSafari:
      (~ua.indexOf("safari") && !~ua.indexOf("chrome")) ||
      Object.prototype.toString
        .call(window.HTMLElement)
        .indexOf("Constructor") > 0,
    isIOS: /iPad|iPhone|iPod/.test(navigator.userAgent) && !window.MSStream,
    isChrome: !!window.chrome && !isOpera
  };
}

function check(uri, failCb, successCb, unsupportedCb) {
  function failCallback() {
    failCb && failCb();
  }

  function successCallback() {
    successCb && successCb();
  }

  if (navigator.msLaunchUri) {
    //for IE and Edge in Win 8 and Win 10
    openUriWithMsLaunchUri(uri, failCb, successCb);
  } else {
    var browser = checkBrowser();

    if (browser.isChrome || browser.isIOS) {
      openUriWithTimeoutHack(uri, failCallback, successCallback);
    } else if (browser.isSafari || browser.isFirefox) {
      openUriWithHiddenFrame(uri, failCallback, successCallback);
    } else {
      unsupportedCb();
      //not supported, implement please
    }
  }
}

(function(window) {
  let url = window.location.href;
  let queryString = url.split("?")[1];
  let params = new URLSearchParams(queryString);

  let redirect = params.get("redirect_uri");

  check(redirect, function() {
    document.getElementById("handle_deeplink").value = "false";
  });
})(window);

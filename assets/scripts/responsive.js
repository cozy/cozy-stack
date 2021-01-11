;(function (d) {
  if (d.body.clientWidth > 1024) {
    const avatars = d.getElementsByClassName('c-avatar')
    for (const avatar of avatars) {
      avatar.classList.add('c-avatar--xlarge')
    }
  } else {
    // XXX u-dn-s has a priority lower than u-flex in Cozy-UI...
    const hidden = d.getElementsByClassName('u-dn-s')
    for (const h of hidden) {
      h.classList.remove('u-flex')
    }
  }
})(document)

;(function (d) {
  // On android, change the address bar color to match the page background
  const paperColor = getComputedStyle(d.body).getPropertyValue(
    '--paperBackgroundColor'
  )
  if (paperColor) {
    const themeColor = d.querySelector('meta[name=theme-color]')
    themeColor.setAttribute('content', paperColor)
  }
})(document)

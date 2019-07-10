;(function(w, d) {
  const data = d.currentScript.dataset
  const tracker = w.Piwik.getTracker(data.matomoUrl, data.matomoSiteId)
  tracker.enableHeartBeatTimer()
  tracker.setUserId(w.location.hostname)
  tracker.setCustomDimension(data.matomoAppId, 'Onboarding')
  tracker.setCustomUrl('/password')
  tracker.trackPageView()
})(window, document)

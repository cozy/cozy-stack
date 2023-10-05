#!/usr/bin/env ruby

require_relative 'boot'

at_exit { Helpers.cleanup }
Helpers.scenario "interactive"
Helpers.start_mailhog

Pry.start binding, quiet: true

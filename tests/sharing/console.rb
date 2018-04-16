#!/usr/bin/env ruby

require_relative 'boot'
require 'pry'

Helpers.scenario "interactive"
Helpers.start_mailhog

AwesomePrint.pry!
Pry.config.history.file = File.expand_path "../tmp/.pry_history", __FILE__
Pry.start binding, prompt: Pry::SIMPLE_PROMPT, quiet: true

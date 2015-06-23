require 'aruba/cucumber'

Before do
	steps %{
		Given a file named "provider" with mode "777" and with:
			"""
			#!/bin/sh
			
			"""
	}
end

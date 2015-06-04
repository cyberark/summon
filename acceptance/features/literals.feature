Feature: literals

	In order to specify related environment variables in one place
	As a developer using Cauldron
	I want to be able to specify literal values in the secrets file

	Scenario: setting an application name
		Given a file named "secrets.yml" with:
			"""
			APPLICATION_NAME: test-app
			"""
		When I successfully run `cauldron -p ./provider env`
		Then the output should contain "APPLICATION_NAME=test-app"

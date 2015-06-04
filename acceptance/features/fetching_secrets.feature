Feature: fetching secrets

	In order to protect secrets
	As a developer using Cauldron
	I want to be able to fetch the externally
	stored secrets into the environment

	Scenario: Fetching a database password
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""

		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `cauldron -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"

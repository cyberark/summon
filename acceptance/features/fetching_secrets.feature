Feature: fetching secrets

	In order to protect secrets
	As a developer using summon
	I want to be able to fetch the externally
	stored secrets into the environment

	Scenario: Fetching a database password
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""

		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"

	Scenario: Fetching a database username and non existent password
		Given a file named "secrets.yml" with:
			"""
			DB_USERNAME: !var very/secret/db-username
			DB_PASSWORD: !var very/secret/db-password-non-existent
			"""
		And a secret "very/secret/db-username" with "secretUsername"
		And a non-existent secret "very/secret/db-password-non-existent"
		When I run `summon -p ./provider env`
		Then the output should contain "Error fetching variable DB_PASSWORD"

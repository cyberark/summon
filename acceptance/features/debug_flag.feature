Feature: debug flag

	In order to troubleshoot issues
	As a developer using summon
	I want to be able to enable debug logging to see detailed information

	Scenario: Debug output is shown with the short flag
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""
		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon -d -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"
		And the output should contain "level=DEBUG"
		And the output should contain "Loading summon configuration"
		And the output should contain "Fetching secrets"

	Scenario: Debug output is shown with the long flag
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""
		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon --debug -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"
		And the output should contain "level=DEBUG"
		And the output should contain "Loading summon configuration"
		And the output should contain "Fetching secrets"

	Scenario: No debug output without the flag
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""
		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"
		And the output should not contain "level=DEBUG"
		And the output should not contain "Loading summon configuration"
		And the output should not contain "Fetching secrets"

	Scenario: Debug output includes secret name on fetch error
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password-non-existent
			"""
		And a non-existent secret "very/secret/db-password-non-existent"
		When I run `summon -d -p ./provider env`
		Then the output should contain "level=DEBUG"
		And the output should contain "Error fetching secret"
		And the output should contain "name=DB_PASSWORD"

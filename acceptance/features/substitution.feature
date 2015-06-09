Feature: substitution

	Scenario: Choosing secrets based on environment
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var env/$ENVIRONMENT/db-password
			"""

		And a secret "env/production/db-password" with "notSoSecret"
		And other secrets don't exist

		When I successfully run `summon -D ENVIRONMENT=production -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"

	Scenario: Storing a deployement parameter in a variable
		Given a file named "secrets.yml" with:
			"""
			RAILS_ENV: $ENV
			"""

		When I successfully run `cauldron -D ENV=production -p ./provider env`
		Then the output should contain "RAILS_ENV=production"

	Scenario: Quoting literal dollars
		Given a file named "secrets.yml" with:
			"""
			PROFIT: $$ $$ $$
			"""

		When I successfully run `cauldron -p ./provider env`
		Then the output should contain "PROFIT=$ $ $"

	Scenario: Unrecognized variables
		Given a file named "secrets.yml" with:
			"""
			RAILS_ENV: $ENVIRONMENT
			"""

		When I run `cauldron -D ENV=production -p ./provider env`
		Then the exit status should not be 0

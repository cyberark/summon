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

    When I successfully run `summon -D ENV=production -p ./provider env`
    Then the output should contain "RAILS_ENV=production"

  Scenario: Quoting literal dollars
    Given a file named "secrets.yml" with:
      """
      PROFIT: $$ $$ $$
      """

    When I successfully run `summon -p ./provider env`
    Then the output should contain "PROFIT=$ $ $"

  Scenario: Unrecognized variables
    Given a file named "secrets.yml" with:
      """
      RAILS_ENV: $ENVIRONMENT
      """

    When I run `summon -D ENV=production -p ./provider env`
    Then the exit status should not be 0

  Scenario: Weird variable contents
    Given a file named "secrets.yml" with:
      """
      MY_DEFINE: $DEFINE
      """

    When I successfully run `summon -D DEFINE=foo=bar -p ./provider env`
    Then the output should contain "MY_DEFINE=foo=bar"

  Scenario: Weird names
    Given a file named "secrets.yml" with:
      """
      WHAT_IS_$ENV: $ENV
      """

    When I successfully run `summon -D ENV=world -p ./provider env`
    Then the output should contain "WHAT_IS_$ENV=world"
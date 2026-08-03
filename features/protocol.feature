Feature: Protocol records
  The parser exposes one ordered model for requests, notices, and replies.

  Scenario: Parse a request with observation limits
    When I parse this record:
      """
      request object.observe count=2 timeout=30
      """
    Then parsing succeeds
    And the record kind is "request"
    And the command is "object.observe"
    And field "count" is "2"
    And field "timeout" is "30"

  Scenario: Parse an unsolicited notice
    When I parse this record:
      """
      notice system.boot storage state=ready
      """
    Then parsing succeeds
    And the record kind is "notice"
    And the command is "system.boot"
    And field "state" is "ready"

  Scenario: Parse a successful terminal
    When I parse this record:
      """
      ok object.status schema=1 count=0 state=ready
      """
    Then parsing succeeds
    And field "schema" is "1"
    And field "count" is "0"
    And field "state" is "ready"

  Scenario Outline: Reject malformed protocol input
    When I parse this record:
      """
      <line>
      """
    Then parsing fails

    Examples:
      | line                                                     |
      | cmd object.status                                        |
      | request  object.status                                   |
      | request object.set value="ready"                         |
      | request object.set value=a\b                             |
      | notice system.boot                                      |
      | event object.observe                                     |
      | ok object.status                                         |
      | ok object.status count=0 schema=1                        |
      | ok object.status schema=01 count=0                       |
      | ok object.status schema=1 count=0 state=ready state=again |
      | err object.status field=mode code=BAD_RESPONSE           |
      | request object.status ..                                 |
      | request object.status alpha.beta.gamma                   |
      | request object.status -                                  |
      | request object.status :::::::                            |

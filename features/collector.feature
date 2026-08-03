Feature: Reply collection
  A collector extracts one command reply from interleaved console output.

  Scenario: Collect a reply
    Given I collect replies for "object.status"
    When these lines arrive:
      """
      unrelated console output
      item object.status alpha state=ready
      event other.observe state=waiting
      item object.status beta state=idle
      ok object.status schema=1 count=2
      """
    Then line collection succeeds
    And the reply is complete
    And the reply schema is 1
    And the reply contains 2 data records

  Scenario: Ignore a matching notice while collecting a reply
    Given I collect replies for "object.status"
    When these lines arrive:
      """
      notice object.status lifecycle state=ready
      item object.status alpha state=ready
      ok object.status schema=1 count=1
      """
    Then line collection succeeds
    And the reply is complete
    And the reply contains 1 data records

  Scenario: Reject a mismatched trailer count
    Given I collect replies for "object.status"
    When these lines arrive:
      """
      item object.status alpha state=ready
      ok object.status schema=1 count=2
      """
    Then line collection succeeds
    And reply validation fails with "declares 2 data records, collected 1"

  Scenario: Return a remote error
    Given I collect replies for "object.status"
    When these lines arrive:
      """
      item object.status alpha state=partial
      err object.status code=IO_ERROR operation=read
      """
    Then line collection succeeds
    And the remote error code is "IO_ERROR"

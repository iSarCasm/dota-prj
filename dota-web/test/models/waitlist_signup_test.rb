require "test_helper"

class WaitlistSignupTest < ActiveSupport::TestCase
  test "normalizes email" do
    signup = WaitlistSignup.create!(email: "  Player@Example.COM ")
    assert_equal "player@example.com", signup.email
  end

  test "requires unique email" do
    WaitlistSignup.create!(email: "player@example.com")
    duplicate = WaitlistSignup.new(email: "player@example.com")

    assert_not duplicate.valid?
    assert_includes duplicate.errors[:email], "has already been taken"
  end
end

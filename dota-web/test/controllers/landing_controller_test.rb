require "test_helper"

class LandingControllerTest < ActionDispatch::IntegrationTest
  test "should get index" do
    get root_url
    assert_response :success
    assert_select ".landing-brand", /Dota/
  end

  test "should join waitlist with valid email" do
    assert_difference -> { WaitlistSignup.count }, 1 do
      post waitlist_url, params: { waitlist_signup: { email: "player@example.com" } }
    end

    assert_redirected_to root_path(joined: 1)
  end

  test "duplicate email still succeeds for the user" do
    WaitlistSignup.create!(email: "player@example.com")

    assert_no_difference -> { WaitlistSignup.count } do
      post waitlist_url, params: { waitlist_signup: { email: "player@example.com" } }
    end

    assert_redirected_to root_path(joined: 1)
  end

  test "rejects invalid email" do
    assert_no_difference -> { WaitlistSignup.count } do
      post waitlist_url, params: { waitlist_signup: { email: "not-an-email" } }
    end

    assert_response :unprocessable_entity
  end
end

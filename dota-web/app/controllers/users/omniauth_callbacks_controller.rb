class Users::OmniauthCallbacksController < Devise::OmniauthCallbacksController
  # OmniAuth callbacks are initiated by a third-party redirect (Steam/OpenID),
  # so they won't include Rails' CSRF token. When auth fails, Devise routes
  # through `#failure`, so that must be exempt too.
  skip_before_action :verify_authenticity_token, only: %i[steam failure]

  def steam
    @user = User.from_omniauth(request.env["omniauth.auth"])

    if @user.persisted?
      sign_in_and_redirect @user, event: :authentication
      set_flash_message(:notice, :success, kind: "Steam") if is_navigational_format?
    else
      session["devise.steam_data"] = request.env["omniauth.auth"].except(:extra)
      redirect_to new_user_registration_url
    end
  end

  def failure
    error = request.env["omniauth.error"]
    error_type = request.env["omniauth.error.type"]
    strategy = request.env["omniauth.error.strategy"]

    binding.irb

    Rails.logger.error "Steam auth failure: Error: #{error.inspect}, Type: #{error_type}, Strategy: #{strategy}"
    Rails.logger.error "Request env: #{request.env.select { |k,v| k.start_with?('omniauth') }.inspect}"
    redirect_to root_path
  end
end
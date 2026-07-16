class LandingController < ApplicationController
  layout "landing"

  def index
    @waitlist_signup = WaitlistSignup.new
    @joined = params[:joined].present?
  end

  def create
    @waitlist_signup = WaitlistSignup.new(waitlist_signup_params)

    if @waitlist_signup.save
      @joined = true
      respond_to do |format|
        format.turbo_stream
        format.html { redirect_to root_path(joined: 1) }
      end
    elsif email_already_joined?
      @joined = true
      @waitlist_signup = WaitlistSignup.new
      respond_to do |format|
        format.turbo_stream { render :create }
        format.html { redirect_to root_path(joined: 1) }
      end
    else
      render :index, status: :unprocessable_entity
    end
  end

  private

  def waitlist_signup_params
    params.require(:waitlist_signup).permit(:email)
  end

  def email_already_joined?
    email = waitlist_signup_params[:email].to_s.strip.downcase
    WaitlistSignup.exists?(email: email)
  end
end

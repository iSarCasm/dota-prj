class HomeController < ApplicationController
  def index
    redirect_to matches_path if user_signed_in?
  end
end

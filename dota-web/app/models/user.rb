class User < ApplicationRecord
  devise :database_authenticatable, :registerable,
         :recoverable, :rememberable, :validatable,
         :confirmable, :omniauthable, omniauth_providers: [:steam]

  def self.from_omniauth(auth)
    user = where(steam_id: auth.uid).first_or_create do |user|
      user.email = "#{auth.uid}@steam.generated"
      user.password = Devise.friendly_token[0, 20]
      user.steam_name = auth.info.nickname
      user.steam_avatar_url = auth.info.image
      user.skip_confirmation!
    end

    # Update steam info if it changed
    if user.steam_name != auth.info.nickname || user.steam_avatar_url != auth.info.image
      user.update(
        steam_name: auth.info.nickname,
        steam_avatar_url: auth.info.image
      )
    end

    user
  end
end

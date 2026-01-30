require_relative "boot"

require "rails/all"

# Require the gems listed in Gemfile, including any gems
# you've limited to :test, :development, or :production.
Bundler.require(*Rails.groups)

module DotaWeb
  class Application < Rails::Application
    # Initialize configuration defaults for originally generated Rails version.
    config.load_defaults 8.1

    # Please, add to the `ignore` list any other `lib` subdirectories that do
    # not contain `.rb` files, or that should not be reloaded or eager loaded.
    # Common ones are `templates`, `generators`, or `middleware`, for example.
    config.autoload_lib(ignore: %w[assets tasks constants])

    # Load all JSON files under `lib/constants` into Rails config.
    #
    # Usage:
    # - Rails.configuration.x.constants.heroes
    # - Rails.configuration.x.constants.patchnotes
    # config.x.constants = ActiveSupport::OrderedOptions.new

    constants_dir = Rails.root.join("lib", "constants")
    Dir.glob(constants_dir.join("*.json").to_s).sort.each do |path|
      key = File.basename(path, ".json").to_sym
      config.x.constants[key] = JSON.parse(File.read(path))
    end

    # Configuration for the application, engines, and railties goes here.
    #
    # These settings can be overridden in specific environments using the files
    # in config/environments, which are processed later.
    #
    # config.time_zone = "Central Time (US & Canada)"
    # config.eager_load_paths << Rails.root.join("extras")
  end
end

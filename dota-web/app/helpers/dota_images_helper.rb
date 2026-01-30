module DotaImagesHelper
  IMAGE_EXTENSIONS = [".png", ".webp", ".jpg", ".jpeg", ".gif", ".svg"].freeze

  # Render a local image from app/assets/images by "display name".
  #
  # Example:
  #   dota_named_image_tag("Anti-Mage", folders: ["hero_icons"])
  #   dota_named_image_tag("Blink Dagger", folders: ["item_icons", "neutral_item_icons"])
  #
  # If no matching file exists, returns nil (so callers can skip rendering).
  def dota_named_image_tag(display_name, folders:, **image_tag_options)
    path = dota_named_image_path(display_name, folders: folders)
    return nil if path.blank?

    image_tag(path, **image_tag_options)
  end

  def dota_named_image_path(display_name, folders:)
    return nil if display_name.blank?

    name = display_name.to_s.strip
    candidates = [
      name,
      name.tr("’", "'"),
      name.tr("'", "’")
    ].uniq

    folders.each do |folder|
      candidates.each do |candidate|
        IMAGE_EXTENSIONS.each do |ext|
          rel = File.join(folder, "#{candidate}#{ext}")
          abs = Rails.root.join("app", "assets", "images", rel)
          return rel if File.exist?(abs)
        end
      end
    end

    nil
  end
end


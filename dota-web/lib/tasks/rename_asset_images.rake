require "fileutils"
require "find"

namespace :assets do
  desc <<~DESC
    Rename icon images under app/assets/images/*icons* by removing noisy suffixes.

    Default behavior is DRY RUN (no changes).

    Examples:
      bin/rails "assets:rename_icons"                     # dry-run
      APPLY=1 bin/rails "assets:rename_icons"             # actually rename
      DIRS=hero_ability_icons,hero_icons APPLY=1 bin/rails "assets:rename_icons"
      REMOVE="dota2 gameasset,dota2 wikiasset,abilityicon" APPLY=1 bin/rails "assets:rename_icons"

    Env:
      APPLY=1            -> perform renames (otherwise dry-run)
      DIRS=...           -> comma-separated directory names under app/assets/images (default: all *icons* dirs)
      REMOVE=...         -> comma-separated phrases to remove (default: "dota2 gameasset,dota2 wikiasset,icon,mapicon,abilityicon,itemicon")
      KEEP_UNDERSCORES=1 -> do not convert "_" to spaces (default: convert)
      VERBOSE=1          -> print each planned/applied rename
  DESC
  task rename_icons: :environment do
    images_root = Rails.root.join("app/assets/images")

    dirs =
      if ENV["DIRS"].to_s.strip != ""
        ENV["DIRS"].split(",").map(&:strip).reject(&:empty?)
      else
        Dir.children(images_root).select do |entry|
          path = images_root.join(entry)
          File.directory?(path) && entry.include?("icons")
        end
      end

    remove_phrases =
      if ENV["REMOVE"].to_s.strip != ""
        ENV["REMOVE"].split(",").map(&:strip).reject(&:empty?)
      else
        [
          "dota2 gameasset",
          "dota2 wikiasset",
          "icon",
          "mapicon",
          "abilityicon",
          "itemicon"
        ]
      end

    remove_regexes = remove_phrases.map do |phrase|
      tokens = phrase.split(/[ _-]+/).reject(&:empty?)
      # Remove only whole "tokens" (bounded by start/end or separators), so we don't accidentally remove
      # substrings inside real names (e.g. the "icon" inside "bionic").
      inner = tokens.map { |t| Regexp.escape(t) }.join(/[ _-]+/.source)
      /(?:\A|[ _-]+)#{inner}(?=\z|[ _-]+)/i
    end

    apply = ENV["APPLY"].to_s == "1"
    verbose = ENV["VERBOSE"].to_s == "1"
    keep_underscores = ENV["KEEP_UNDERSCORES"].to_s == "1"

    planned = []
    collisions = []
    skipped = 0

    dirs.each do |dir_name|
      root = images_root.join(dir_name)
      unless File.directory?(root)
        warn "Skipping missing directory: #{root}"
        next
      end

      Find.find(root) do |path|
        next unless File.file?(path)

        ext = File.extname(path)
        base = File.basename(path, ext)

        new_base = base.dup
        remove_regexes.each do |rx|
          new_base.gsub!(rx, " ")
        end

        new_base.tr!("_", " ") unless keep_underscores
        new_base.gsub!(/\s+/, " ")
        new_base.gsub!(/_+/, "_") if keep_underscores
        new_base.gsub!(/-+/, "-")
        new_base.strip!
        new_base.gsub!(/\A[_-]+/, "")
        new_base.gsub!(/[_-]+\z/, "")

        next if new_base == base
        if new_base.empty?
          skipped += 1
          warn "Skipping (would become empty): #{path}"
          next
        end

        new_path = File.join(File.dirname(path), "#{new_base}#{ext}")

        if File.exist?(new_path)
          collisions << [path, new_path]
          next
        end

        planned << [path, new_path]
      end
    end

    if planned.empty? && collisions.empty?
      puts "No renames needed."
      next
    end

    puts "Planned renames: #{planned.length}"
    puts "Collisions (skipped): #{collisions.length}"
    puts "Other skipped: #{skipped}"
    puts "Mode: #{apply ? "APPLY" : "DRY RUN"}"
    puts "Remove phrases: #{remove_phrases.join(", ")}"

    collisions.first(25).each do |from, to|
      warn "Collision: #{from} -> #{to} (target already exists)"
    end
    if collisions.length > 25
      warn "...and #{collisions.length - 25} more collisions"
    end

    planned.each do |from, to|
      puts "#{apply ? "RENAMING" : "WOULD RENAME"}: #{from} -> #{to}" if verbose
      FileUtils.mv(from, to) if apply
    end

    puts(apply ? "Done." : "Dry run complete. Re-run with APPLY=1 to rename.")
  end
end


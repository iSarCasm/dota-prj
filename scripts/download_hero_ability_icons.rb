#!/usr/bin/env ruby
# frozen_string_literal: true

require 'net/http'
require 'uri'
require 'fileutils'
require 'json'

# Configuration
BASE_URL = 'https://liquipedia.net/commons'
MAIN_CATEGORY_URL = "#{BASE_URL}/Category:Dota_2_hero_ability_icons"
OUTPUT_DIR = 'hero_ability_icons'
DELAY_BETWEEN_REQUESTS = 0.5 # Be polite to the server

# User agent for requests
USER_AGENT = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36'

def create_output_dir(dir)
  FileUtils.mkdir_p(dir) unless Dir.exist?(dir)
end

def fetch_url(url)
  uri = URI(url)
  http = Net::HTTP.new(uri.host, uri.port)
  http.use_ssl = true
  http.read_timeout = 30

  request = Net::HTTP::Get.new(uri.request_uri)
  request['User-Agent'] = USER_AGENT

  response = http.request(request)

  case response
  when Net::HTTPSuccess
    response.body
  else
    puts "Error fetching #{url}: #{response.code} #{response.message}"
    nil
  end
rescue StandardError => e
  puts "Error fetching #{url}: #{e.message}"
  nil
end

def extract_hero_subcategories(html_content)
  hero_categories = []

  # Extract subcategory links
  # Pattern: /commons/Category:Dota_2_HeroName_ability_icons
  html_content.scan(%r{/commons/Category:Dota_2_([^"'\s<>]+)_ability_icons}i) do |match|
    hero_name_part = match[0]
    # Reconstruct the full category name
    category_name = "Category:Dota_2_#{hero_name_part}_ability_icons"
    hero_categories << category_name unless hero_categories.include?(category_name)
  end

  # Also try to find links in href attributes
  html_content.scan(/href=["']([^"']*Category:Dota_2_[^"']*_ability_icons[^"']*)["']/i) do |match|
    href = match[0]
    if href.include?('Category:Dota_2_') && href.include?('_ability_icons')
      category_name = href.split('Category:').last.split('"').first.split("'").first.split('?').first.split('#').first
      hero_categories << category_name unless hero_categories.include?(category_name)
    end
  end

  hero_categories.uniq.sort
end

def extract_hero_name_from_category(category_name)
  # Extract hero name from "Category:Dota_2_Dazzle_ability_icons" -> "dazzle"
  # Or "Category:Dota_2_Queen_of_Pain_ability_icons" -> "queen_of_pain"
  if category_name =~ /Category:Dota_2_(.+?)_ability_icons/
    hero_name = $1.downcase
    # Convert spaces and special characters to underscores
    hero_name.gsub(/[^a-z0-9_]/, '_').gsub(/_+/, '_')
  else
    nil
  end
end

def get_files_via_api(category_title)
  # Use MediaWiki API to get all files from category
  api_url = "#{BASE_URL}/api.php"
  file_names = []
  continue_token = nil

  loop do
    params = {
      'action' => 'query',
      'format' => 'json',
      'list' => 'categorymembers',
      'cmtitle' => category_title,
      'cmtype' => 'file',
      'cmlimit' => '500' # Max per request
    }

    params['cmcontinue'] = continue_token if continue_token

    uri = URI(api_url)
    uri.query = URI.encode_www_form(params)

    response_body = fetch_url(uri.to_s)
    break unless response_body

    begin
      data = JSON.parse(response_body)
      query = data.dig('query')
      break unless query

      members = query['categorymembers']
      break unless members

      members.each do |member|
        title = member['title']
        # Extract filename from "File:Name.png"
        if title.start_with?('File:')
          file_name = title[5..-1] # Remove "File:" prefix
          # Filter for abilityicon files
          if file_name.include?('abilityicon') && file_name.include?('dota2') && file_name.end_with?('.png')
            file_names << file_name
          end
        end
      end

      # Check for continuation
      continue_token = data.dig('continue', 'cmcontinue')
      break unless continue_token

      sleep(DELAY_BETWEEN_REQUESTS) # Be polite between API requests
    rescue JSON::ParserError => e
      puts "  Error parsing API response: #{e.message}"
      break
    end
  end

  file_names.uniq.sort
end

def extract_file_links(html_content)
  file_links = []

  # Extract File: links from the HTML
  # Pattern: /commons/File:HeroName_AbilityName_abilityicon_dota2_gameasset.png
  html_content.scan(%r{/commons/File:([^"'\s<>]+\.png)}i) do |match|
    file_name = match[0]
    # Filter for abilityicon files
    if file_name.include?('abilityicon') && file_name.include?('dota2')
      file_links << file_name unless file_links.include?(file_name)
    end
  end

  # Also try to find links in href attributes
  html_content.scan(/href=["']([^"']*File:[^"']*\.png[^"']*)["']/i) do |match|
    href = match[0]
    if href.include?('File:') && href.include?('abilityicon') && href.include?('dota2')
      file_name = href.split('File:').last.split('?').first.split('#').first
      file_links << file_name unless file_links.include?(file_name)
    end
  end

  file_links.uniq.sort
end

def get_image_url_via_api(file_name)
  # Use MediaWiki API to get the direct image URL
  api_url = "#{BASE_URL}/api.php"
  params = {
    'action' => 'query',
    'format' => 'json',
    'titles' => "File:#{file_name}",
    'prop' => 'imageinfo',
    'iiprop' => 'url'
  }

  uri = URI(api_url)
  uri.query = URI.encode_www_form(params)

  response_body = fetch_url(uri.to_s)
  return nil unless response_body

  begin
    data = JSON.parse(response_body)
    pages = data.dig('query', 'pages')
    return nil unless pages

    pages.each_value do |page|
      imageinfo = page.dig('imageinfo')
      next unless imageinfo && imageinfo.first

      return imageinfo.first['url']
    end
  rescue JSON::ParserError => e
    puts "    Error parsing API response: #{e.message}"
  end

  nil
end

def construct_image_url(file_name)
  # Fallback: construct URL based on MediaWiki's file organization
  "#{BASE_URL}/images/#{file_name[0].downcase}/#{file_name[0..1].downcase}/#{file_name}"
end

def download_image(image_url, file_path)
  image_data = fetch_url(image_url)
  return false unless image_data

  # Check if it's actually an image (starts with PNG/JPEG magic bytes)
  unless image_data.start_with?("\x89PNG".b) || image_data.start_with?("\xFF\xD8\xFF".b)
    puts "    Warning: Response doesn't appear to be an image"
  end

  File.binwrite(file_path, image_data)
  true
rescue StandardError => e
  puts "    ✗ Error saving: #{e.message}"
  false
end

def download_hero_abilities(hero_category, hero_name)
  hero_dir = File.join(OUTPUT_DIR, hero_name)
  create_output_dir(hero_dir)

  puts "  Fetching ability icons for #{hero_name}..."

  # Try API first
  file_names = get_files_via_api(hero_category)

  # Fallback to HTML parsing
  if file_names.empty?
    category_url = "#{BASE_URL}/#{hero_category.gsub(' ', '_')}"
    html_content = fetch_url(category_url)

    if html_content
      file_names = extract_file_links(html_content)
    end
  end

  if file_names.empty?
    puts "  No ability icons found for #{hero_name}"
    return 0
  end

  puts "  Found #{file_names.length} ability icons for #{hero_name}"

  successful = 0
  failed = 0

  file_names.each_with_index do |file_name, index|
    # Try API first to get the correct image URL
    image_url = get_image_url_via_api(file_name)

    # Fallback to constructed URL if API fails
    image_url ||= construct_image_url(file_name)

    file_path = File.join(hero_dir, file_name)

    if download_image(image_url, file_path)
      successful += 1
      puts "    [#{index + 1}/#{file_names.length}] ✓ #{file_name}"
    else
      failed += 1
      puts "    [#{index + 1}/#{file_names.length}] ✗ #{file_name}"
    end

    # Be polite to the server
    sleep(DELAY_BETWEEN_REQUESTS) if index < file_names.length - 1
  end

  puts "  Completed #{hero_name}: #{successful} successful, #{failed} failed"
  successful
end

def main
  puts "Starting Dota 2 hero ability icon downloader..."
  puts "Source: #{MAIN_CATEGORY_URL}"
  puts

  # Create main output directory
  create_output_dir(OUTPUT_DIR)
  puts

  # Fetch the main category page
  puts "Fetching main category page..."
  html_content = fetch_url(MAIN_CATEGORY_URL)

  unless html_content
    puts "Failed to fetch main category page. Exiting."
    exit 1
  end

  # Extract hero subcategories
  puts "Extracting hero subcategories..."
  hero_categories = extract_hero_subcategories(html_content)

  if hero_categories.empty?
    puts "No hero subcategories found. The page structure might have changed."
    exit 1
  end

  puts "Found #{hero_categories.length} hero subcategories"
  puts

  # Download abilities for each hero
  total_successful = 0
  total_failed = 0

  hero_categories.each_with_index do |hero_category, index|
    hero_name = extract_hero_name_from_category(hero_category)

    unless hero_name
      puts "[#{index + 1}/#{hero_categories.length}] Could not extract hero name from: #{hero_category}"
      total_failed += 1
      next
    end

    puts "[#{index + 1}/#{hero_categories.length}] Processing: #{hero_name}"

    successful = download_hero_abilities(hero_category, hero_name)
    total_successful += successful

    # Be polite to the server between heroes
    sleep(DELAY_BETWEEN_REQUESTS) if index < hero_categories.length - 1
  end

  # Summary
  puts
  puts '=' * 60
  puts 'Download complete!'
  puts "  Heroes processed: #{hero_categories.length}"
  puts "  Total ability icons downloaded: #{total_successful}"
  puts "  Output directory: #{File.expand_path(OUTPUT_DIR)}"
  puts '=' * 60
end

main if __FILE__ == $PROGRAM_NAME

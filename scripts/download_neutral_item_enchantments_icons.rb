#!/usr/bin/env ruby
# frozen_string_literal: true

require 'net/http'
require 'uri'
require 'fileutils'
require 'json'

# Configuration
BASE_URL = 'https://liquipedia.net/commons'
CATEGORY_URL = "#{BASE_URL}/Category:Dota_2_neutral_item_enchantments_icons"
OUTPUT_DIR = 'neutral_item_enchantments_icons'
DELAY_BETWEEN_REQUESTS = 0.5 # Be polite to the server

# User agent for requests
USER_AGENT = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36'

def create_output_dir
  FileUtils.mkdir_p(OUTPUT_DIR) unless Dir.exist?(OUTPUT_DIR)
  puts "Created directory: #{OUTPUT_DIR}"
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

def get_files_via_api(category_title)
  # Use MediaWiki API to get all files from category (handles pagination automatically)
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
          # Filter for enchantment itemicon files
          if file_name.include?('itemicon') && file_name.include?('dota2') && file_name.end_with?('.png')
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
  # Pattern: /commons/File:EnchantmentName_Enchantment_itemicon_dota2_gameasset.png
  html_content.scan(%r{/commons/File:([^"'\s<>]+\.png)}i) do |match|
    file_name = match[0]
    # Filter for enchantment itemicon files
    if file_name.include?('itemicon') && file_name.include?('dota2')
      file_links << file_name unless file_links.include?(file_name)
    end
  end

  # Also try to find links in href attributes
  html_content.scan(/href=["']([^"']*File:[^"']*\.png[^"']*)["']/i) do |match|
    href = match[0]
    if href.include?('File:') && href.include?('itemicon') && href.include?('dota2')
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
    puts "  Error parsing API response: #{e.message}"
  end

  nil
end

def construct_image_url(file_name)
  # Fallback: construct URL based on MediaWiki's file organization
  # MediaWiki stores files in subdirectories based on filename hash
  # Format: /commons/images/X/Xy/filename.png
  "#{BASE_URL}/images/#{file_name[0].downcase}/#{file_name[0..1].downcase}/#{file_name}"
end

def download_image(image_url, file_name)
  puts "  Downloading: #{file_name}"

  image_data = fetch_url(image_url)
  return false unless image_data

  # Check if it's actually an image (starts with PNG/JPEG magic bytes)
  unless image_data.start_with?("\x89PNG".b) || image_data.start_with?("\xFF\xD8\xFF".b)
    puts "  Warning: Response doesn't appear to be an image"
    # Still try to save it, might be HTML error page
  end

  file_path = File.join(OUTPUT_DIR, file_name)
  File.binwrite(file_path, image_data)

  file_size = File.size(file_path)
  puts "  ✓ Saved: #{file_name} (#{file_size} bytes)"
  true
rescue StandardError => e
  puts "  ✗ Error saving #{file_name}: #{e.message}"
  false
end

def main
  puts "Starting Dota 2 neutral item enchantment icon downloader..."
  puts "Source: #{CATEGORY_URL}"
  puts

  # Create output directory
  create_output_dir
  puts

  # Try to get all files via API first (handles pagination automatically)
  puts "Fetching file list via MediaWiki API..."
  file_names = get_files_via_api('Category:Dota_2_neutral_item_enchantments_icons')

  # Fallback to HTML parsing if API doesn't work
  if file_names.empty?
    puts "API method returned no files. Trying HTML parsing..."
    html_content = fetch_url(CATEGORY_URL)

    unless html_content
      puts "Failed to fetch category page. Exiting."
      exit 1
    end

    file_names = extract_file_links(html_content)

    if file_names.empty?
      puts "No file links found. The page structure might have changed."
      exit 1
    end

    puts "Note: HTML parsing may not get all files due to pagination."
    puts "Consider using the API method for complete results."
  end

  puts "Found #{file_names.length} neutral item enchantment icon files"
  puts

  # Download images
  puts "Downloading images to '#{OUTPUT_DIR}' directory..."
  puts

  successful = 0
  failed = 0

  file_names.each_with_index do |file_name, index|
    puts "[#{index + 1}/#{file_names.length}] Processing: #{file_name}"

    # Try API first to get the correct image URL
    image_url = get_image_url_via_api(file_name)

    # Fallback to constructed URL if API fails
    image_url ||= construct_image_url(file_name)

    if download_image(image_url, file_name)
      successful += 1
    else
      failed += 1
    end

    # Be polite to the server
    sleep(DELAY_BETWEEN_REQUESTS) if index < file_names.length - 1
  end

  # Summary
  puts
  puts '=' * 60
  puts 'Download complete!'
  puts "  Successful: #{successful}"
  puts "  Failed: #{failed}"
  puts "  Total: #{file_names.length}"
  puts "  Output directory: #{File.expand_path(OUTPUT_DIR)}"
  puts '=' * 60
end

main if __FILE__ == $PROGRAM_NAME

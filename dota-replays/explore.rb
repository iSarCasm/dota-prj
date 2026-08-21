#!/usr/bin/env ruby
# frozen_string_literal: true

# Discover recent public matches via OpenDota and download replays into
# explored/{rank}/{match_id}/{match_id}.{dem.bz2,dem,json}

require "json"
require "fileutils"
require "optparse"
require "time"
require "set"

require_relative "lib/opendota"
require_relative "lib/replay"
require_relative "lib/ranks"
require_relative "lib/heroes"

ROOT = File.expand_path(__dir__)
EXPLORED = File.join(ROOT, "explored")
TURBO_MODE = 23
MAX_PAGES = 40
FAILED_MATCHES = Set.new

options = {
  rank: nil,
  hero: nil,
  limit: nil,
  turbo: false,
  min_duration: 600,
  min_age: 20,
  all_heroes: false
}

parser = OptionParser.new do |opts|
  opts.banner = "Usage: #{$PROGRAM_NAME} [options]"
  opts.on("--rank NAME", "Medal filter (#{Ranks.names.join(', ')}); omit for all") { |v| options[:rank] = v }
  opts.on("--hero NAME_OR_ID", "Hero filter (name or id); omit for any") { |v| options[:hero] = v }
  opts.on("--all-heroes", "Download until every hero appears in explored/") { options[:all_heroes] = true }
  opts.on("--limit N", Integer, "Max downloads (default: 1; with --all-heroes: 200)") { |v| options[:limit] = v }
  opts.on("--turbo", "Include turbo matches (game_mode 23)") { options[:turbo] = true }
  opts.on("--min-duration SEC", Integer, "Skip shorter matches (default: 600)") { |v| options[:min_duration] = v }
  opts.on("--min-age MIN", Integer, "Skip matches newer than N minutes (default: 20; Valve 404s)") { |v| options[:min_age] = v }
  opts.on("-h", "--help", "Show help") do
    puts opts
    exit
  end
end
parser.parse!

if options[:all_heroes] && options[:hero]
  warn "[explore] --all-heroes and --hero are mutually exclusive"
  exit 1
end

options[:limit] ||= options[:all_heroes] ? 200 : 1
options[:turbo] = true if options[:all_heroes] # rare heroes need the denser turbo sample

begin
  hero_id = options[:hero] ? Heroes.resolve(options[:hero]) : nil
  min_rank, max_rank = options[:rank] ? Ranks.min_max_for(options[:rank]) : [nil, nil]
rescue ArgumentError => e
  warn "[explore] #{e.message}"
  exit 1
end

def match_heroes(match)
  (Array(match["radiant_team"]) + Array(match["dire_team"])).map(&:to_i).reject(&:zero?)
end

def usable_match?(match, hero_id:, turbo:, min_duration:, min_age:)
  mid = match["match_id"]
  return false if FAILED_MATCHES.include?(mid)
  return false if match["duration"].to_i < min_duration
  return false if !turbo && match["game_mode"].to_i == TURBO_MODE

  started = match["start_time"].to_i
  return false if started <= 0
  return false if Time.now.to_i - started < min_age * 60

  heroes = match_heroes(match)
  return false if heroes.length < 10
  return false if hero_id && !heroes.include?(hero_id.to_i)

  true
end

def already_have?(rank_name, match_id)
  return true if FAILED_MATCHES.include?(match_id)

  dem = File.join(EXPLORED, rank_name, match_id.to_s, "#{match_id}.dem")
  Replay.valid_dem?(dem)
end

def download_match!(public_match, filters:)
  match_id = public_match["match_id"]
  rank_name = Ranks.name_for(public_match["avg_rank_tier"])
  dir = File.join(EXPLORED, rank_name, match_id.to_s)
  dem_path = File.join(dir, "#{match_id}.dem")
  bz2_path = File.join(dir, "#{match_id}.dem.bz2")
  json_path = File.join(dir, "#{match_id}.json")

  begin
    details = Replay.fetch_into(
      match_id,
      dem_path: dem_path,
      bz2_path: bz2_path,
      keep_bz2: true,
      log_prefix: "[explore]"
    )
  rescue StandardError => e
    warn "[explore] #{match_id}: #{e.message}; trying next"
    FileUtils.rm_f(bz2_path)
    FileUtils.rm_rf(dir) if Dir.exist?(dir) && (Dir.children(dir) rescue []).empty?
    FAILED_MATCHES << match_id
    return nil
  end

  if details.nil?
    FAILED_MATCHES << match_id
    return nil
  end
  return :skipped if details == :skipped

  meta = {
    "public_match" => public_match,
    "match" => details,
    "filters" => filters,
    "downloaded_at" => Time.now.utc.iso8601
  }
  File.write(json_path, JSON.pretty_generate(meta))
  puts "[explore] wrote #{json_path}"
  details
end

def scan_public_matches(min_rank:, max_rank:, turbo:, min_duration:, min_age:, hero_id: nil, max_results: nil)
  seen = {}
  less_than = nil
  results = []

  MAX_PAGES.times do |page|
    batch = OpenDota.get_public_matches(
      min_rank: min_rank,
      max_rank: max_rank,
      less_than_match_id: less_than
    )
    break if batch.nil? || batch.empty?

    batch.each do |match|
      mid = match["match_id"]
      next if seen[mid]

      seen[mid] = true
      next unless usable_match?(
        match,
        hero_id: hero_id,
        turbo: turbo,
        min_duration: min_duration,
        min_age: min_age
      )

      rank_name = Ranks.name_for(match["avg_rank_tier"])
      next if already_have?(rank_name, mid)

      results << match
      return results if max_results && results.length >= max_results
    end

    less_than = batch.map { |m| m["match_id"] }.min
    break if less_than.nil?

    sleep 1
    puts "[explore] page #{page + 1}: scanned=#{seen.size} candidates=#{results.size} (less_than=#{less_than})"
  end

  results
end

def pick_best_for_missing(candidates, missing)
  candidates
    .map { |m| [m, (match_heroes(m).to_set & missing).size] }
    .select { |_, n| n.positive? }
    .max_by { |m, n| [n, m["duration"].to_i] }
    &.first
end

def run_normal(options, hero_id:, min_rank:, max_rank:)
  want = [options[:limit] * 5, options[:limit] + 4].max
  puts "[explore] rank=#{options[:rank] || 'all'} hero=#{options[:hero] || 'any'}" \
       " (id=#{hero_id || '-'}) limit=#{options[:limit]} turbo=#{options[:turbo]} min_age=#{options[:min_age]}m"

  candidates = scan_public_matches(
    min_rank: min_rank,
    max_rank: max_rank,
    turbo: options[:turbo],
    min_duration: options[:min_duration],
    min_age: options[:min_age],
    hero_id: hero_id,
    max_results: want
  )

  if candidates.empty?
    warn "[explore] no matching public matches found"
    exit 1
  end

  puts "[explore] trying up to #{candidates.length} candidate(s) for #{options[:limit]} download(s)"
  downloaded = 0
  filters = {
    "rank" => options[:rank],
    "hero" => options[:hero],
    "hero_id" => hero_id,
    "turbo" => options[:turbo],
    "mode" => "normal"
  }

  candidates.each do |public_match|
    break if downloaded >= options[:limit]

    details = download_match!(public_match, filters: filters)
    downloaded += 1 if details.is_a?(Hash)
  end

  if downloaded.zero?
    warn "[explore] failed to download any replay"
    exit 1
  end

  puts "[explore] downloaded #{downloaded}/#{options[:limit]}"
end

def run_all_heroes(options, min_rank:, max_rank:)
  all_ids = Heroes.all_ids.to_set
  names = Heroes.id_to_name
  covered = Heroes.covered_in(EXPLORED).keys.to_set
  missing = all_ids - covered

  puts "[explore] --all-heroes coverage #{covered.size}/#{all_ids.size} " \
       "(missing #{missing.size}) limit=#{options[:limit]} turbo=#{options[:turbo]} " \
       "rank=#{options[:rank] || 'all'} min_age=#{options[:min_age]}m"

  if missing.empty?
    puts "[explore] already have every hero"
    return
  end

  puts "[explore] missing: #{missing.map { |id| names[id] || id }.sort.join(', ')}"

  downloaded = 0
  filters = {
    "rank" => options[:rank],
    "turbo" => options[:turbo],
    "mode" => "all_heroes"
  }

  while !missing.empty? && downloaded < options[:limit]
    target = missing.min
    puts "[explore] need #{missing.size} hero(s); seeking #{names[target] || target} (#{target})"

    pool = scan_public_matches(
      min_rank: min_rank,
      max_rank: max_rank,
      turbo: options[:turbo],
      min_duration: options[:min_duration],
      min_age: options[:min_age],
      max_results: 200
    )

    unless pool.any? { |m| match_heroes(m).include?(target) }
      puts "[explore] scanning specifically for hero #{target}"
      pool.concat(
        scan_public_matches(
          min_rank: min_rank,
          max_rank: max_rank,
          turbo: options[:turbo],
          min_duration: options[:min_duration],
          min_age: options[:min_age],
          hero_id: target,
          max_results: 50
        )
      )
    end

    # Dedupe by match_id
    pool = pool.uniq { |m| m["match_id"] }

    best = pick_best_for_missing(pool, missing)
    if best.nil?
      warn "[explore] stuck: no candidate covers remaining heroes " \
           "(#{missing.map { |id| names[id] || id }.sort.join(', ')})"
      exit 1
    end

    gained = match_heroes(best).to_set & missing
    puts "[explore] pick #{best['match_id']} covers +#{gained.size}: " \
         "#{gained.map { |id| names[id] || id }.sort.join(', ')}"

    details = download_match!(
      best,
      filters: filters.merge("target_heroes" => gained.to_a.sort)
    )

    if details.is_a?(Hash)
      downloaded += 1
      covered = Heroes.covered_in(EXPLORED).keys.to_set
      missing = all_ids - covered
      puts "[explore] coverage now #{covered.size}/#{all_ids.size} (downloaded #{downloaded})"
    else
      FAILED_MATCHES << best["match_id"]
    end
  end

  if missing.empty?
    puts "[explore] complete: every hero covered (#{downloaded} new download(s))"
  else
    warn "[explore] unfinished: #{covered.size}/#{all_ids.size}; still missing " \
         "#{missing.map { |id| names[id] || id }.sort.join(', ')}"
    exit 1
  end
end

if options[:all_heroes]
  run_all_heroes(options, min_rank: min_rank, max_rank: max_rank)
else
  run_normal(options, hero_id: hero_id, min_rank: min_rank, max_rank: max_rank)
end

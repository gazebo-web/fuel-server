#!/usr/bin/env ruby

require 'nokogiri'
require 'formdata'
require 'optparse'
require 'json'

# The world file used to create thumbnails
worldSDF = <<-WORLD
<sdf version='1.6'>
  <world name='default'>
    <scene>
      <ambient>0.5 0.5 0.5 1</ambient>
      <background>.980392157 .980392157 .980392157 1</background>
      <shadows>0</shadows>
      <grid>0</grid>
    </scene>
    <gui fullscreen='0'>
      <camera name='user_camera'>
        <pose>5.65634 -4.1009 2.6069 0 0.275643 2.35619</pose>
        <view_controller>orbit</view_controller>
      </camera>
    </gui>
  </world>
</sdf>
WORLD
worldFile = Tempfile.new('whiteworld')
worldFile.write(worldSDF)
worldFile.close

options = {}

# Check that IGN_FUEL_JWT is set
if !ENV.key?('IGN_FUEL_JWT')
  puts "IGN_FUEL_JWT environment variable is not set. Set IGN_FUEL_JWT to a value JWT authentication token."
  exit
end

# Parse command line options
OptionParser.new do |opts|
  opts.on('-u url', '--url', String, 'Destination URL. Such as https://api.ignitionfuel.org') do |u|
    options['url'] = u
  end

  opts.on('-d dir', '--dir', String, 'Directory of gazebo models. This option should be the full path to a directory containing one or more gazebo models.') do |d|
   options['dir'] = d
  end

  opts.on('-o owner', '--owner', String, 'Name of the owner.') do |o|
   options['owner'] = o
  end

end.parse!

# Check for the existence of a destination server
if !options.key?('url')
  puts "Missing destination URL. Use the -u command line option."
  exit
end

# Check for the existence of an owner
if !options.key?('owner')
  puts "Missing owner information. Use the -o command line option."
  exit
end

# Check for the existence of a source directory
if !options.key?('dir')
  puts "Missing source directory. Use the -d command line option."
  exit
end

uri = URI(options['url'])

# Iterate over each directory under the provided directory
Dir.foreach(options['dir']) do |item|

  # Get the complete filename of the next file
  filename = File.join(options['dir'], item)

  # Make sure the file is a directory and not a special file
  if File.directory?(filename) && item != "." && item[0] != "."
    puts "# Processing #{filename}"

    # Open the model.config
    begin
      doc = File.open("#{filename}/model.config") { |f| Nokogiri::XML(f) }
    rescue
      puts "  ! Failed to open #{filename}/model.config."
      next
    end

    # Get the <model> element
    begin
      model = doc.at_xpath('model')
      if model == nil
        raise
      end
    rescue
      puts "  ! Error reading <model> element in #{filename}/model.config. Skipping."
      next
    end

    # Read the <name> value
    begin
      name =  model.at_xpath('name').content
      name.strip!
      if name == nil || name.gsub(/[[:space:]]/,'').empty?
        raise
      end
    rescue
      puts "  ! Missing or empty <name> element in #{filename}/model.config. Skipping."
      next
    end

    # Read the <description> value
    begin
      description =  model.at_xpath('description').content
      description.strip!
      if description == nil || description.gsub(/[[:space:]]/,'').empty?
        raise
      end
    rescue
      puts "  ! Warning. Missing <description> in #{filename}/model.config."
    end

    sdfFile = ""
    # Read the <sdf> values and get the max version
    begin
      maxVersion = 0.0

      model.xpath('sdf').each do |s|
        version = s.attr("version").to_f
        if version > maxVersion
          sdfFile = s.content
          maxVersion = version
        end
      end
    rescue
      puts "  ! Unable to read the <sdf> element."
    end

    thumbdir = File.join(filename, "thumbnails")
    modelSDF = File.join(filename, sdfFile)
    if !File.exist?(modelSDF)
      puts "  ! #{sdfFile} file does not exist. Skipping"
      next
    end
    
    begin  
      FileUtils.rmdir(thumbdir)
    rescue
    end

    begin
      cmd = "gzserver -s libModelPropShop.so #{worldFile.path} --propshop-save '#{thumbdir}' --propshop-model '#{modelSDF}' 2>/dev/null"
      if !system(cmd)
        raise
      else
        puts "  Created thumbnails"
      end
    rescue
      puts "  ! Failed to create thumbnails"
    end

    # Fill out the form data
    formData = FormData.new
    formData.append('multipart', true)
    formData.append('name', name)
    formData.append('URLName', name.tr(" ", "_"))
    formData.append('description', description)
    formData.append('tags', '')
    formData.append('license', '1')
    formData.append('owner', options['owner'])

    formData.append('permission', '0')
    formData.append('private', '0')

    # Append all the model files
    Dir.glob("#{filename}/**/*").each {|i|
      if !File.directory?(i) && item != "." && item[0] != "."
        pathname = File.dirname(i)
        pathname.slice!(options['dir'])
        formData.append('file',File.open(i, 'rb'), {
          :filename => "#{pathname}/#{File.basename(i)}"
        })
      end
    }

    # Send the request.
    begin
      req = Net::HTTP::Post.new("/1.0/models")
      req['Authorization'] = "Bearer #{ENV['IGN_FUEL_JWT']}"
      req.content_type = formData.content_type
      req.content_length = formData.size
      req.body_stream = formData

      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      res = http.request(req)
      if res.code != "200"
        raise
      else
        puts "  Uploaded"
      end

      # Don't abuse the server too much.
      sleep 2
    rescue
      errMsg = ""

      if res != nil && res.body != nil
        begin
          errMsg = JSON.parse(res.body)['msg']
        rescue
          errMsg = ""
        end
      end

      puts "  Failed to upload #{filename}.\n\t #{errMsg}"
    end
  end
end

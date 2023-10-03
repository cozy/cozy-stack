class Debug
  def self.visualize_tree(instances, sharing)
    @count ||= 0
    Visualizer.new(instances, sharing, @count).render
    @count += 1
  end

  class Visualizer
    def initialize(instances, sharing, suffix)
      @instances = instances
      @sharing = sharing
      @output = File.join Helpers.current_dir, "tree_#{suffix}"
    end

    def render
      File.write "#{@output}.gv", to_dot
      system "dot -Tpng -o #{@output}.png #{@output}.gv"
      system "xdg-open #{@output}.png"
    end

    def to_dot
      @buf = []
      @buf << 'digraph debug {'
      @buf << '  node [shape="box", fontname="lato", fontsize=10, margin=0.12, color="#297EF2", fontcolor="#32363F"];'
      @buf << '  edge [color="#32363F"];'
      @buf << '  ranksep=0.4; nodesep=0.5;'

      @instances.each_with_index do |inst, idx|
        @buf << '' << "  subgraph cluster_#{inst.name} {"
        @buf << "    label=\"#{inst.name}\"; labeljust=\"l\"; fontname=\"lato\"; fontsize=12" << ''
        @buf << "    #{esc Folder::ROOT_DIR, inst} [label=\"/\"]"
        if idx == 0
          @buf << "    invisible [style=invis]"
          @buf << "    invisible -> #{esc Folder::ROOT_DIR, inst} [style=invis]"
        end
        tree_for(inst).sort_by { |doc| doc['name'] || '' }.each do |doc|
          next if ignored? doc
          @buf << "    #{esc doc['_id'], inst} [label=<#{doc['name']}<BR/>#{doc['_id']}<BR/>#{doc['_rev']}>]"
          @buf << "    #{esc doc['dir_id'], inst} -> #{esc doc['_id'], inst}"
        end
        @buf << '  }'
      end

      @buf << '}' << ''
      @buf.join "\n"
    end

    def ignored?(doc)
      return true if doc["_id"].start_with?("_") # Design docs
      return true if doc["_id"] == Folder::ROOT_DIR
      path = doc["path"]
      return false if path.nil?
      return true if path.start_with?("/Photos")
      return true if path.start_with?("/Administrati")
      false
    end

    # Escape identifiers for graphviz
    def esc(id, inst)
      case id
      when Folder::ROOT_DIR then "#{inst.name}_root"
      when Folder::TRASH_DIR then "#{inst.name}_trash"
      when Folder::SHARED_WITH_ME_DIR then "#{inst.name}_shared"
      when Folder::NO_LONGER_SHARED_DIR then "#{inst.name}_no_longer"
      else 'x' + id
      end
    end

    def tree_for(inst)
      Helpers.couch.all_docs inst.domain, CozyFile.doctype
    end
  end
end

<?php

class WPIException extends \Exception {

}

class WPI
{
    const STATE_PREPARE = 0;
    const STATE_UPLOADING = 1;

    const EOP = "\n";

    private $server;

    private $path;

    private $token;

    private $metaFile;

    private $sock;

    public function __construct( $server, $path )
    {
        $this->server = $server;
        $this->path = realpath($path);

        if( false === $this->path ) {
            throw new RuntimeException("Path is invalid");
        }

        $this->metaFile = $this->path . DIRECTORY_SEPARATOR . "wpide.imp";
        ob_implicit_flush();
        set_time_limit(0);
    }

    /**
     * @param $token
     */
    public function setToken($token) {
        $this->token = $token;
    }

    /**
     * @return boolean
     * @throws
     */
    public function start( $progress )
    {
        $this->connect();

        $sent       = 0;
        $imported   = 0;
        $meta       = $this->prepare();
        $total      = count($meta['files']);

        foreach ($meta['info'] as $name => $value) {
            $this->write("SET " . $name . " " . $value);
            if(!$this->isOK()) {
                throw new WPIException("Fail to set ".$name." property");
            }
        }

        $this->write("SET files " . count($meta['files']));

        if(!$this->isOK()) {
            throw new WPIException("Fail to set number of files");
        }

        foreach($meta['files'] as $index => $file) {

            if( $file['state'] === 0 ) {

                $sent++;

                if($this->import($file)){
                    $meta['files'][$index]['state'] = 1;
                    $this->saveMeta($meta);
                    $imported++;
                }

                if( $progress ) {
                    call_user_func_array(
                        $progress,
                        array(
                            $file['path'],
                            round($sent / $total * 100)
                        )
                    );
                }
            }
        }

        $this->write("FINISH");

        $response = stream_get_contents($this->sock, 7);

        if( $response !== "FINISH\n" ) {
            throw new WPIException("Fail to finish");
        }

        fclose($this->sock);

        return $imported === $sent;
    }

    private function connect() {

        $this->sock = @stream_socket_client("tcp://" . $this->server, $errorNumber, $errorMessage);

        if ($this->sock === false) {
            throw new UnexpectedValueException("Failed to connect: $errorMessage");
        }

        $this->write("AUTH " . $this->token);

        if( !$this->isOK() ) {
            throw new WPIException("Authentication failed");
        }

    }

    private function write( $data, $eol = true ) {
        if( $eol ) {
            $data = $data . self::EOP;
        }
        fwrite($this->sock, $data, strlen($data));
    }

    private function isOK() {
        $string = stream_get_contents($this->sock, 3);
        return $string === "OK\n";
    }

    /**
     * Get all files from wordpress directory
     * @param $dir
     * @param $nodes
     * @return array
     */
    private function getFilesRecursive( $dir, &$nodes = array() ) {

        if($handler = opendir($dir))
        {
            while (($sub = readdir($handler)) !== FALSE)
            {
                $exclude = array(".","..","Thumb.db");

                if(in_array($sub, $exclude)) {
                    continue;
                }

                $node = array(
                    'state' => 0,
                    'path'  => $dir."/".$sub
                );

                $isDir = is_dir($dir."/".$sub);

                if( $isDir ) {
                    $this->getFilesRecursive($dir."/".$sub, $nodes);
                } else {
                    $nodes[] = $node;
                }
            }

            closedir($handler);
        }

        return $nodes;
    }

    private function prepare() {

        if(!file_exists($this->path) || !is_dir($this->path)) {
            throw new UnexpectedValueException("Invalid path provided: " . $this->path);
        }

        require $this->path . DIRECTORY_SEPARATOR . "wp-load.php";

        global $wpdb;

        $this->dumpSQL($wpdb);

        $meta = array();

        $meta['info'] = array(
            'name' => get_bloginfo('name'),
            'url' => get_bloginfo('url'),
            'admin_email' => get_bloginfo('admin_email'),
            'version' => get_bloginfo('version'),
        );

        $meta['files'] = $this->getFilesRecursive($this->path);
        $meta['state'] = self::STATE_PREPARE;

        $this->saveMeta($meta);

        return $meta;
    }

    /**
     * Save metadata
     * @param $meta
     */
    private function saveMeta($meta) {
        return;
        $json = json_encode($meta);

        if( false === $json ) {
            throw new RuntimeException("Fail to encode meta array");
        }

        if( false === file_put_contents($this->metaFile, $json) ) {
            throw new RuntimeException("Fail to write wpi file");
        }
    }

    /**
     * @param $file
     * @return array
     * @throws Error
     * @throws WPIException
     */
    private function import( $file )
    {
        $relative = str_replace($this->path . '/', '', $file['path']);

        $this->write("IMPORT " . $relative . "|" . filesize($file['path']));

        if ( !$this->isOK() ) {
            return false;
        }

        if (!file_exists($file['path'])) {
            unlink($this->path . DIRECTORY_SEPARATOR . "wpi");
            throw new WPIException("Meta may be corrupted, file to import missing.");
        }

        $handle = fopen($file['path'], "r");
        $contents = fread($handle, filesize($file['path']));

        $this->write($contents);
        $this->write("END");

        return $this->isOK();
    }

    /**
     * @param $wpdb
     * @throws WPIException
     */
    private function dumpSQL($wpdb)
    {
        $sqlFile = $this->path . DIRECTORY_SEPARATOR . "db.sql";

        if (file_exists($sqlFile)) {
            unlink($sqlFile);
        }

        $tables = $wpdb->get_results('show tables', ARRAY_N);

        $fp = fopen($sqlFile, "a");

        if ($fp === false) {
            throw new WPIException("Fail to write database sql file");
        }

        $header = "-- DATABASE DUMP " . date('r') . "\n";

        fwrite($fp, $header, strlen($header));

        foreach ($tables as $table) {

            $sql = "";

            foreach ($wpdb->get_results('show create table ' . $table[0], ARRAY_N) as $create) {
                $sql = $sql . "-- " . $create[0] . "\n\n" . $create[1] . ";\n\n\n";
            }

            fwrite($fp, $sql, strlen($sql));


            $data = $wpdb->get_results("SELECT * FROM " . $table[0], ARRAY_A);

            $sql = "";
            foreach ($data as $row) {
                $sql .= "INSERT INTO `" . $table[0] . "` SET ";
                $parts = array();
                foreach ($row as $field => $value) {
                    $parts[] = "`" . $field . "` = '" . esc_sql($value) . "'";
                }
                $sql .= implode($parts, ', ') . ";\n";
            }

            $sql .= "\n\n\n";

            fwrite($fp, $sql, strlen($sql));
        }

        fclose($fp);
    }
}
<?php

class WPIException extends \Exception {}

class WPI
{

    const RESPONSE_OK = 0;

    const STATE_PREPARE = 0;
    const STATE_UPLOADING = 1;
    const CHUNK_SIZE = 1024;

    const EOP = "\n";

    private $server;

    private $path;

    private $token;

    private $metaFile;

    private $sock;

    private $verboseEnabled = false;

    private $dbtempfile;

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

    public function setVerbose($state) {
        $this->verboseEnabled = $state;
    }

    /**
     * @return boolean
     * @throws
     */
    public function start( $progress = null )
    {
        $meta       = $this->prepare();
        $total      = count($meta['files']);

        foreach ($meta['info'] as $name => $value) {
            $this->write("SET " . $name . " " . $value);
            if( $this->getResponseCode() != self::RESPONSE_OK) {
                throw new WPIException("Fail to set ".$name." property");
            }
        }

        $this->write("SET files " . count($meta['files']));

        if( $this->getResponseCode() != self::RESPONSE_OK) {
            throw new WPIException("Fail to set number of files");
        }

        if(!file_exists($this->dbtempfile)) {
            throw new WPIException("Sql dump file missing");
        }

        foreach($meta['files'] as $index => $file) {

            if( $file['state'] === 0 ) {

                $relative = str_replace($this->path . '/', '', $file['path']);

                if( $this->import($file['path'], $relative) !== true ) {
                    throw new WPIException("Fail to import " . $relative);
                }

                $meta['files'][$index]['state'] = 1;
                $this->saveMeta($meta);

                if( $progress ) {
                    call_user_func_array(
                        $progress,
                        array(
                            str_replace($this->path . '/', '', $file['path']),
                            round($sent / $total * 100)
                        )
                    );
                }
            }
        }

        $this->import($this->dbtempfile, "db.sql");

        $this->write("FINISH");

        $response = stream_get_contents($this->sock);
        fclose($this->sock);

        $json = json_decode($response,true);

        if( $json === false ) {
            throw new WPIException("Finish response must be a json. This means that is not successful. The actual response is " . $response);
        }

        return $response;
    }

    /**
     * @param $file
     * @param $relative
     * @return array
     * @throws Error
     * @throws WPIException
     */
    private function import( $file, $relative )
    {

        if (!file_exists($file)) {
            unlink($this->path . DIRECTORY_SEPARATOR . "wpi");
            throw new WPIException("Meta may be corrupted, file to import missing.");
        }

        $crc = hash_file("md5", $file);

        $this->verbose("Import " . $relative);

        $this->write("IMPORT " . $relative . "|" . filesize($file) . "|" . $crc);

        $response = $this->getResponseCode();

        if ( $response == 2 ) {
            //already exist
            return true;
        }

        if ( $response != self::RESPONSE_OK ) {
            return false;
        }

        $finfo = finfo_open(FILEINFO_MIME);
        $ascii = substr(finfo_file($finfo, $file), 0, 4) == 'text';
        finfo_close($finfo);

        $this->verbose("Read mode: " . ($ascii?"ascii":"binary"));

        $handle = fopen($file, $ascii ? "r" : "rb");

        $count=0;

        while(!feof($handle)) {
            $buffer = fread($handle, self::CHUNK_SIZE);
            $this->write($buffer, false);
            fflush($handle);
            $count++;
        }

        $response = $this->getResponseCode();

        if( $response != self::RESPONSE_OK ) {

            if( $response === 9 ) {
                throw new WPIException("Server restarted. Please try again!");
            }

            throw new WPIException("Fail to transfer file");
        }

        $this->verbose("Write " . $count . " chunks");

        return true;
    }

    private function verbose($msg) {
        if($this->verboseEnabled) {
            echo $msg.PHP_EOL;
        }
    }

    public function connect() {

        $this->sock = @stream_socket_client("tcp://" . $this->server, $errorNumber, $errorMessage);

        if ($this->sock === false) {
            throw new UnexpectedValueException("Failed to connect to wpide import server " . $this->server);
        }

        $this->write("AUTH " . $this->token);

        $response = $this->getResponseCode();

        if( $response == 2) {
            throw new WPIException("Token expired. Please generate other token from the application.");
        }

        if( $response == 3) {
            throw new WPIException("This token scope is not for import. Please generate other token from the application.");
        }

        if( $response != 0) {
            throw new WPIException("Token is invalid. Please generate other token from the application.");
        }
    }

    private function write( $data, $eol = true ) {
        if( $eol ) {
            $data = $data . self::EOP;
        }
        fwrite($this->sock, $data, strlen($data));
    }

    private function getResponseCode() {
        $string = stream_get_contents($this->sock, 2);
        $string = trim(preg_replace('/\s\s+/', ' ', $string));
        return intval($string);
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
     * @param $wpdb
     * @throws WPIException
     */
    private function dumpSQL($wpdb)
    {
        $this->dbtempfile = tempnam(get_temp_dir(),"wpide_import_");

        if (file_exists($sqlFile)) {
            unlink($sqlFile);
        }

        $tables = $wpdb->get_results('show tables', ARRAY_N);

        $fp = fopen($this->dbtempfile, "a");

        if ($fp === false) {
            throw new WPIException("Fail to write database sql file");
        }

        $header = "";

        fwrite($fp, $header, strlen($header));

        foreach ($tables as $table) {

            $sql = "";

            foreach ($wpdb->get_results('show create table ' . $table[0], ARRAY_N) as $create) {
                $sql = $sql . preg_replace("/\n/","",$create[1]) . ";\n";
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

            fwrite($fp, $sql, strlen($sql));
        }

        fclose($fp);
    }
}
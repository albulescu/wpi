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

    public function __construct( $server, $path )
    {
        $this->server = $server;
        $this->path = $path;
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

    /**
     * Prepare file to be passed to the socket
     * @param $force
     * @return array
     */
    private function prepare( $force = false ) {

        if(!file_exists($this->path) || !is_dir($this->path)) {
            throw new UnexpectedValueException("Invalid path provided: " . $this->path);
        }

        if( $force === false  && file_exists($this->metaFile)) {

            $meta = json_decode(file_get_contents($this->metaFile), true);

            if( $meta === null ) {
                unlink($this->metaFile);
                throw new UnexpectedValueException("Meta should not be null. The wpi file removed, please try again!");
            }

            return $meta;
        }

        if(!file_exists($this->path . DIRECTORY_SEPARATOR . "wp-load.php")) {
            throw new RuntimeException("Invalid WordPress path, wp-load.php missing");
        }

        require $this->path . DIRECTORY_SEPARATOR . "wp-load.php";

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
     * @param bool $force Force building metadata
     * @return boolean
     * @throws WPIException
     */
    public function start( $force = false )
    {
        $start = microtime(true);

        $meta = $this->prepare($force);

        $client = @stream_socket_client("tcp://" . $this->server, $errorNumber, $errorMessage);

        if ($client === false) {
            throw new UnexpectedValueException("Failed to connect: $errorMessage");
        }

        fwrite($client, "AUTH " . $this->token . self::EOP);

        if( stream_get_contents($client,1) === "0" ) {
            throw new WPIException("Authentication failed");
        }

        $sent = 0;
        $imported = 0;

        foreach($meta['files'] as $index => $file) {
            if( $file['state'] === 0 ) {

                echo "Import {$file['path']}\n";

                $relative = str_replace($this->path . '/', '', $file['path']);

                fwrite($client, "IMPORT " . $relative . "|" . filesize($file['path']) . self::EOP);

                if( stream_get_contents($client, 1) === "0" ) {
                    // file already imported
                    continue;
                }

                if(!file_exists($file['path'])) {
                    unlink($this->path . DIRECTORY_SEPARATOR . "wpi");
                    throw new WPIException("Meta may be corrupted file to import missing.");
                }

                $sent++;

                $handle = fopen($file['path'], "r");
                $contents = fread($handle, filesize($file['path']));

                fwrite($client, $contents);
                fwrite($client, "END" . self::EOP);

                if( stream_get_contents($client,1) === "1" ) {
                    $meta['files'][$index]['state'] = 1;
                    $this->saveMeta($meta);
                    $imported++;
                } else {
                    throw new Error("Fail to import file");
                }
            }
        }

        fclose($client);

        $time_elapsed_secs = microtime(true) - $start;

        die("Imported $imported files from $sent in ". $time_elapsed_secs . "\n");

        return $imported === $sent;
    }
}